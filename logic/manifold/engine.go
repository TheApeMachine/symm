package manifold

import (
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/signal/compute"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

const (
	IdealGasGamma           = 5.0 / 3.0
	DefaultBaselineHalflife = 30 * time.Second
	BaselineEpsilon         = 1e-9
	SizeBins                = 16
	AgeBins                 = 16
)

/*
Engine owns one Metal initialization context and per-symbol solver slots.
*/
type Engine struct {
	config   *pmanifold.Config
	slots    *sync.Map
	maxSlots int
	halflife time.Duration
	epsilon  float64
}

func NewEngine() *Engine {
	bookDepth := viper.GetViper().GetInt("market.l3_depth")
	halflife := viper.GetViper().GetDuration("market.baseline_halflife")
	maxSlots := viper.GetViper().GetInt("market.manifold_max_symbols")

	if halflife <= 0 {
		halflife = DefaultBaselineHalflife
	}

	if maxSlots <= 0 {
		maxSlots = 64
	}

	config := &pmanifold.Config{
		GridX:    uint32(bookDepth),
		GridY:    SizeBins,
		GridZ:    AgeBins,
		DomainX:  float64(bookDepth),
		DomainY:  float64(SizeBins),
		DomainZ:  float64(AgeBins),
		DeltaT:   types.Unit,
		Gamma:    IdealGasGamma,
		MaxModes: 32,
	}

	pmanifold.ApplyDerivedGasParams(config)
	pmanifold.DefaultMarketGasBoundaries().Apply(config)

	return &Engine{
		config:   config,
		slots:    &sync.Map{},
		maxSlots: maxSlots,
		halflife: halflife,
		epsilon:  BaselineEpsilon,
	}
}

func (engine *Engine) Config() *pmanifold.Config {
	return engine.config
}

func (engine *Engine) Close() {
	engine.slots.Range(func(key, value any) bool {
		slot := value.(*Slot)
		slot.Close()
		return true
	})
}

func (engine *Engine) Admit(symbol string, thesis *strategy.Thesis) (*Slot, error) {
	if found, ok := engine.slots.Load(symbol); ok {
		return found.(*Slot), nil
	}

	count := 0

	engine.slots.Range(func(key, value any) bool {
		count++
		return true
	})

	if count >= engine.maxSlots {
		return nil, errnie.Err(
			errnie.Internal,
			"logic manifold engine: admission capacity exhausted",
			nil,
		)
	}

	slot := newSlot(symbol, thesis, engine.config, engine.halflife, engine.epsilon)

	if err := slot.open(); err != nil {
		return nil, err
	}

	engine.slots.Store(symbol, slot)

	return slot, nil
}

func (engine *Engine) Slot(symbol string) (*Slot, bool) {
	found, ok := engine.slots.Load(symbol)

	if !ok {
		return nil, false
	}

	return found.(*Slot), true
}

/*
ProcessResult carries replay metadata from one L3 ingest step.
*/
type ProcessResult struct {
	Thesis       *strategy.Thesis
	State        State
	Accounting   PopulationAccounting
	CohortCount  int
	OrderCount   int
	DepositCount int
	GasReady     bool
	ReplayPushed bool
}

/*
Slot is one symbol's population, coordinate, cohort, and solver state.
*/
type Slot struct {
	symbol      string
	thesis      *strategy.Thesis
	config      *pmanifold.Config
	population  *Population
	coordinates *CoordinateMapper
	cohorts     *CohortBuilder
	depositor   *MomentDepositor
	modes       *ModeExtractor
	forecaster  *Forecaster
	solver      *pmanifold.Solver
	lastEventAt time.Time
}

func newSlot(
	symbol string,
	thesis *strategy.Thesis,
	config *pmanifold.Config,
	halflife time.Duration,
	epsilon float64,
) *Slot {
	lifetime := NewLifetimeEstimator()

	return &Slot{
		symbol:      symbol,
		thesis:      thesis,
		config:      config,
		population:  NewPopulation(symbol, lifetime),
		coordinates: NewCoordinateMapper(halflife, epsilon, lifetime),
		cohorts:     NewCohortBuilder(64),
		depositor:   NewMomentDepositor(config),
		modes:       NewModeExtractor(config),
		forecaster:  NewForecaster(),
	}
}

func (slot *Slot) Close() {
	if slot.solver != nil {
		slot.solver.Close()
		slot.solver = nil
	}
}

func (slot *Slot) Thesis() *strategy.Thesis {
	return slot.thesis
}

func (slot *Slot) open() error {
	if slot.solver != nil {
		return nil
	}

	return compute.WithMetalInit(func() error {
		solver := pmanifold.NewSolver(*slot.config)

		if solver == nil {
			return errnie.Err(
				errnie.Internal,
				"logic manifold engine: solver was not created",
				nil,
			)
		}

		slot.solver = solver

		return nil
	})
}

func (slot *Slot) Process(
	row kraken.Level3Data,
	pricePrecision int,
	qtyPrecision int,
	book Level3Book,
) ProcessResult {
	result := ProcessResult{Thesis: slot.thesis}

	if book == nil {
		slot.population.invalidate(ChecksumFailed)
		return result
	}

	if !book.Apply(row, pricePrecision, qtyPrecision) {
		slot.population.invalidate(ChecksumFailed)
		return result
	}

	if book.Invalid(slot.symbol) {
		slot.population.invalidate(BookInvalid)
		return result
	}

	bid, ask, ok := book.TopOfBook(slot.symbol)

	if !ok {
		slot.population.invalidate(NonPositiveMid)
		return result
	}

	midPrice := (bid + ask) / 2
	slot.population.Apply(row, midPrice)

	if !slot.population.Ready() {
		return result
	}

	at := row.Timestamp

	if at.IsZero() {
		at = slot.population.LastAt()
	}

	if !slot.lastEventAt.IsZero() && at.Before(slot.lastEventAt) {
		slot.markFailed(at, TimestampRegress, slot.population.Epoch(), slot.coordinates.ScaleVersion())
		return result
	}

	orders := slot.population.Orders()
	transform, epochReady := slot.coordinates.BeginEpoch(orders, midPrice, at)

	if !epochReady {
		slot.markFailed(at, UnmappedCarriers, slot.population.Epoch(), slot.coordinates.ScaleVersion())
		return result
	}

	mapped, mapReady := slot.mapCarriers(orders, midPrice, at, transform)

	if !mapReady {
		slot.markFailed(at, UnmappedCarriers, slot.population.Epoch(), transform.Version)
		return result
	}

	cohorts := slot.cohorts.Build(mapped)
	eventDeltaT := slot.eventDeltaT(at)
	subdivisions := EventSubdivisions(slot.config, eventDeltaT, cohorts)

	if subdivisions <= 0 {
		slot.markFailed(at, StabilityFailed, slot.population.Epoch(), transform.Version)
		return result
	}

	outcome := slot.step(
		cohorts,
		mapped,
		at,
		eventDeltaT,
		subdivisions,
		transform,
		slot.population.Epoch(),
		slot.population.Accounting(),
	)

	if outcome.GasReady {
		slot.lastEventAt = at
	}

	result.Thesis = outcome.Thesis
	result.State = outcome.State
	result.GasReady = outcome.GasReady
	result.Accounting = slot.population.Accounting()
	result.CohortCount = len(cohorts)
	result.OrderCount = len(mapped)
	result.DepositCount = outcome.DepositCount

	return result
}

func (slot *Slot) eventDeltaT(at time.Time) float64 {
	if slot.lastEventAt.IsZero() || at.IsZero() {
		return slot.config.DeltaT
	}

	deltaT := at.Sub(slot.lastEventAt).Seconds()

	if deltaT <= 0 {
		return slot.config.DeltaT
	}

	return deltaT
}

func (slot *Slot) mapCarriers(
	orders []*PhysicalOrder,
	midPrice float64,
	at time.Time,
	transform EpochTransform,
) ([]*PhysicalOrder, bool) {
	mapped := make([]*PhysicalOrder, 0, len(orders))

	for _, order := range orders {
		previous := order.Coordinate
		coordinate, ready := slot.coordinates.MapOrder(order, midPrice, at, transform)

		if !ready {
			return nil, false
		}

		slot.coordinates.UpdateVelocity(order, previous, coordinate, at)
		order.Coordinate = coordinate
		order.MappedAt = at
		mapped = append(mapped, order)
	}

	return mapped, true
}

type stepOutcome struct {
	Thesis       *strategy.Thesis
	State        State
	GasReady     bool
	DepositCount int
}

func (slot *Slot) step(
	cohorts []Cohort,
	orders []*PhysicalOrder,
	at time.Time,
	eventDeltaT float64,
	subdivisions int,
	transform EpochTransform,
	epoch uint64,
	accounting PopulationAccounting,
) stepOutcome {
	state := State{
		At:            at,
		Epoch:         epoch,
		ScaleVersion:  transform.Version,
		Ready:         true,
		InvalidReason: Valid,
		DeltaT:        eventDeltaT,
		Subdivisions:  subdivisions,
		PriceScale:    transform.PriceScale,
		SizeScale:     transform.SizeScale,
	}

	if err := slot.solver.ResetSources(); err != nil {
		errnie.Error(errnie.Err(errnie.UnprocessableContent, "logic manifold: reset sources failed", err))
		return slot.markFailed(at, SolverFailed, epoch, transform.Version)
	}

	deposits := slot.depositor.Deposits(cohorts)

	for _, deposit := range deposits {
		if err := slot.solver.SourceCell(
			deposit.CellX,
			deposit.CellY,
			deposit.CellZ,
			deposit.MomX,
			deposit.MomY,
			deposit.MomZ,
			deposit.Rho,
			deposit.EInt,
		); err != nil {
			errnie.Error(errnie.Err(errnie.UnprocessableContent, "logic manifold: source failed", err))
			return slot.markFailed(at, SolverFailed, epoch, transform.Version)
		}
	}

	substepDeltaT := eventDeltaT / float64(subdivisions)
	oscillators := slot.modes.Modes(cohorts, substepDeltaT)

	if len(oscillators) == 0 {
		oscillators = slot.modes.SpectrumAnchor(cohorts, eventDeltaT)
	}

	if len(oscillators) == 0 {
		return slot.markFailed(at, SolverFailed, epoch, transform.Version)
	}

	if err := slot.solver.SetOscillators(oscillators); err != nil {
		errnie.Error(errnie.Err(errnie.UnprocessableContent, "logic manifold: set oscillators failed", err))
		return slot.markFailed(at, SolverFailed, epoch, transform.Version)
	}

	controls := slot.config.RuntimeControls()
	controls.DeltaT = substepDeltaT
	controls.MetabolicRate = 1 / substepDeltaT

	if err := slot.solver.SetControls(controls); err != nil {
		errnie.Error(errnie.Err(errnie.UnprocessableContent, "logic manifold: set controls failed", err))
		return slot.markFailed(at, SolverFailed, epoch, transform.Version)
	}

	var reading pmanifold.Reading

	for stepIndex := 0; stepIndex < subdivisions; stepIndex++ {
		var err error
		reading, err = slot.solver.Step()

		if err != nil {
			errnie.Error(errnie.Err(errnie.UnprocessableContent, "logic manifold: step failed", err))
			return slot.markFailed(at, SolverFailed, epoch, transform.Version)
		}

		if !reading.IsFinite() {
			return slot.markFailed(at, SolverFailed, epoch, transform.Version)
		}
	}

	visibleMass := slot.depositor.VisibleMass(cohorts)
	pressureTensor := ReferencePressureTensor(slot.config, cohorts)

	state.Reading = reading
	state.VisibleMass = visibleMass
	state.ConservationResidual = slot.depositor.ConservationResidual(accounting, visibleMass)
	state.BidTouchDensity = touchMassDensity(orders, OrderSideBid, transform.PriceScale)
	state.AskTouchDensity = touchMassDensity(orders, OrderSideAsk, transform.PriceScale)
	state.PressureTensor = pressureTensor
	state.StressAnisotropy = stressAnisotropy(pressureTensor)
	state.OscillatorCount = len(oscillators)

	slot.thesis.AddEvidence("manifold", state)
	slot.thesis.AddEvidence("step_at", at)

	gasReady := state.GasReady()

	if gasReady {
		slot.forecaster.Attach(slot.thesis, state)
	}

	return stepOutcome{
		Thesis:       slot.thesis,
		State:        state,
		GasReady:     gasReady,
		DepositCount: len(deposits),
	}
}

func (slot *Slot) markFailed(
	at time.Time,
	reason InvalidReason,
	epoch uint64,
	scaleVersion uint64,
) stepOutcome {
	state := State{
		At:            at,
		Epoch:         epoch,
		ScaleVersion:  scaleVersion,
		Ready:         false,
		InvalidReason: reason,
	}

	slot.thesis.AddEvidence("manifold_invalid", string(reason))
	slot.thesis.AddEvidence("manifold", state)

	return stepOutcome{
		Thesis:   slot.thesis,
		State:    state,
		GasReady: false,
	}
}
