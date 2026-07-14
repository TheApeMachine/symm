package manifold

import (
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/signal/compute"
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
	config           *pmanifold.Config
	forecastConfig   ForecastConfig
	lifetimeCapacity int
	slots            *sync.Map
	maxSlots         int
	halflife         time.Duration
	epsilon          float64
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
		DomainZ:  1,
		DeltaT:   types.Unit,
		Gamma:    IdealGasGamma,
		MaxModes: 32,
	}

	pmanifold.ApplyDerivedGasParams(config)
	pmanifold.DefaultMarketGasBoundaries().Apply(config)

	return &Engine{
		config: config,
		forecastConfig: ForecastConfig{
			InitialVariance:  viper.GetViper().GetFloat64("market.forecast.rls.initial_variance"),
			ForgettingFactor: viper.GetViper().GetFloat64("market.forecast.rls.forgetting_factor"),
		},
		lifetimeCapacity: viper.GetViper().GetInt("market.manifold.lifetime_capacity"),
		slots:            &sync.Map{},
		maxSlots:         maxSlots,
		halflife:         halflife,
		epsilon:          BaselineEpsilon,
	}
}

func (engine *Engine) Config() *pmanifold.Config {
	return engine.config
}

func (engine *Engine) Halflife() time.Duration {
	return engine.halflife
}

func (engine *Engine) Close() {
	engine.slots.Range(func(key, value any) bool {
		slot := value.(*Slot)
		slot.Close()
		return true
	})
}

func (engine *Engine) Admit(symbol string) (*Slot, error) {
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

	slot, err := newSlot(
		symbol,
		engine.config,
		engine.forecastConfig,
		engine.lifetimeCapacity,
		engine.halflife,
		engine.epsilon,
	)

	if err != nil {
		return nil, err
	}

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
	State         State
	Forecast      *types.Forecasts
	Observation   ObservationMetadata
	Accounting    PopulationAccounting
	CohortCount   int
	OrderCount    int
	DepositCount  int
	AdvanceReady  bool
	GasReady      bool
	StateProduced bool
}

/*
Slot is one symbol's population, coordinate, cohort, and solver state.
*/
type Slot struct {
	symbol       string
	config       *pmanifold.Config
	population   *Population
	coordinates  *CoordinateMapper
	cohorts      *CohortBuilder
	depositor    *MomentDepositor
	modes        *ModeExtractor
	forecaster   *ForecastModel
	solver       *pmanifold.Solver
	pending      pendingObservation
	advanceReady bool
	lastEventAt  time.Time
}

func newSlot(
	symbol string,
	config *pmanifold.Config,
	forecastConfig ForecastConfig,
	lifetimeCapacity int,
	halflife time.Duration,
	epsilon float64,
) (*Slot, error) {
	lifetime := NewLifetimeEstimator(lifetimeCapacity)
	forecaster, err := NewForecastModel(forecastConfig)

	if err != nil {
		return nil, err
	}

	return &Slot{
		symbol:      symbol,
		config:      config,
		population:  NewPopulation(symbol, lifetime),
		coordinates: NewCoordinateMapper(halflife, epsilon, lifetime),
		cohorts:     NewCohortBuilder(config),
		depositor:   NewMomentDepositor(config),
		modes:       NewModeExtractor(config),
		forecaster:  forecaster,
	}, nil
}

func (slot *Slot) Close() {
	if slot.solver != nil {
		slot.solver.Close()
		slot.solver = nil
	}
}

func (slot *Slot) failedResult(
	observation ObservationMetadata,
	reason InvalidReason,
) ProcessResult {
	outcome := slot.markFailed(
		observation.At,
		reason,
		slot.population.Epoch(),
		slot.coordinates.ScaleVersion(),
	)

	return ProcessResult{
		State:         outcome.State,
		Observation:   observation,
		Accounting:    slot.population.Accounting(),
		StateProduced: true,
	}
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

		slot.coordinates.UpdateVelocity(order, previous, coordinate, at, transform, midPrice)
		order.Coordinate = coordinate
		order.MappedAt = at
		order.ScaleVersion = transform.Version
		order.ReferenceMid = midPrice
		mapped = append(mapped, order)
	}

	return mapped, true
}

type stepOutcome struct {
	State        State
	Forecast     *types.Forecasts
	GasReady     bool
	DepositCount int
}

func (slot *Slot) step(
	cohorts []Cohort,
	orders []*PhysicalOrder,
	bestBid float64,
	bestAsk float64,
	bestBidQuantity float64,
	bestAskQuantity float64,
	midPrice float64,
	at time.Time,
	eventDeltaT float64,
	subdivisions int,
	transform EpochTransform,
	epoch uint64,
	accounting PopulationAccounting,
) stepOutcome {
	state := State{
		Source:        "manifold",
		Symbol:        slot.symbol,
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
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"logic manifold: step failed: "+err.Error(),
				err,
			))
			return slot.markFailed(at, SolverFailed, epoch, transform.Version)
		}

		if !reading.IsFinite() {
			return slot.markFailed(at, SolverFailed, epoch, transform.Version)
		}
	}

	if err := state.FieldSnapshot.Read(slot.solver); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"logic manifold: field read failed",
			err,
		))
		return slot.markFailed(at, SolverFailed, epoch, transform.Version)
	}

	conservation := slot.depositor.Conservation(accounting, cohorts)
	pressureTensor := ReferencePressureTensor(slot.config, cohorts)
	touchBand := slot.config.DomainX / float64(slot.config.GridX)

	state.Reading = reading
	state.BestBid = bestBid
	state.BestAsk = bestAsk
	state.BestBidQuantity = bestBidQuantity
	state.BestAskQuantity = bestAskQuantity
	state.MidPrice = midPrice
	state.VisibleMass = conservation.VisibleMass
	state.ConservationResidual = conservation.Residual
	state.ConservationBound = conservation.Bound
	state.BidTouchDensity = touchMassDensity(orders, OrderSideBid, touchBand)
	state.AskTouchDensity = touchMassDensity(orders, OrderSideAsk, touchBand)
	state.PressureTensor = pressureTensor
	state.StressAnisotropy = stressAnisotropy(pressureTensor)
	state.OscillatorCount = len(oscillators)

	gasReady := state.GasReady()
	var producedForecast *types.Forecasts

	if gasReady {
		forecast, ready, err := slot.forecaster.Update(state)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"logic manifold: forecast update failed",
				err,
			))
		}

		if ready {
			producedForecast = &forecast
		}
	}

	return stepOutcome{
		State:        state,
		Forecast:     producedForecast,
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
		Source:        "manifold",
		Symbol:        slot.symbol,
		At:            at,
		Epoch:         epoch,
		ScaleVersion:  scaleVersion,
		Ready:         false,
		InvalidReason: reason,
	}

	return stepOutcome{
		State:    state,
		GasReady: false,
	}
}
