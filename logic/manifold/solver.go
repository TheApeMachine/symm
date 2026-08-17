package manifold

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"sync/atomic"
	"time"

	mgrbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/geometry"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/signal/compute"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
phaseLatticeWidth is the pair-carrier census and the ω-bin count of the
universe phase dial. Each subscribed symbol occupies one carrier. Level 3
orders on that pair are oscillators in that carrier's collection.
*/
const phaseLatticeWidth uint32 = 256

/*
HawkesSignal encapsulates the self-exciting event rates extracted from
the symbol's Hawkes excitation measurement.
*/
type HawkesSignal struct {
	LambdaBuy    float64 // Aggressive buy intensity (events/sec)
	LambdaSell   float64 // Aggressive sell intensity (events/sec)
	LambdaCancel float64 // Cancellation intensity (events/sec)
	Reflexivity  float64 // Branching ratio η ∈ [0, 1)
	AvgTradeSize float64 // Mean trade volume
}

/*
Solver owns one resident Sensorium domain for the complete market universe.
Symbols contribute observations to the same gas and wave fields; they are not
split into independent simulations that cannot interfere.
*/
type Solver struct {
	api         websocket.BookSource
	config      pmanifold.Config
	physics     *pmanifold.Solver
	oscillators []pmanifold.Oscillator
	reading     pmanifold.Reading
	recorder    *audit.Recorder
	scales      map[string]*adaptive.Accumulator
	converged   map[string]float64
	priorPos    map[string]map[string][3]float64
	corpus      *geometry.Corpus[types.PhaseOutcome]
	angles      []float64
	pending     []pendingDial
	ui          chan []byte
	binui       chan types.FluidFrame
	closing     atomic.Bool
}

/*
NewSolver creates the single shared Metal domain and a spectral corpus bounded
by the same explicit event-history capacity as the live market feed.
*/
func NewSolver(
	api *websocket.API,
	ui chan []byte,
	binui chan types.FluidFrame,
	recorder *audit.Recorder,
) *Solver {
	deltaT := 0.01
	configuredDelta := viper.GetDuration("market.manifold.integration_interval")

	if configuredDelta > 0 {
		deltaT = configuredDelta.Seconds()
	}

	config, err := pmanifold.NewConfig(
		64,
		64,
		64,
		1,
		32,
		deltaT,
		5.0/3.0,
		phaseLatticeWidth,
	)
	errnie.Error(err)
	config.MaxParticles = 65536
	pmanifold.DefaultMarketGasBoundaries().Apply(&config)
	physics, physicsErr := newPhysics(config)
	errnie.Error(physicsErr)

	corpus, corpusErr := geometry.NewCorpus[types.PhaseOutcome](phaseCorpusCapacity)
	errnie.Error(corpusErr)

	angles, angleErr := geometry.PhasePath(phaseScanAngles)
	errnie.Error(angleErr)

	var bookSource websocket.BookSource
	if api != nil {
		bookSource = api
	}

	return &Solver{
		api:       bookSource,
		config:    config,
		physics:   physics,
		recorder:  recorder,
		scales:    make(map[string]*adaptive.Accumulator),
		converged: make(map[string]float64),
		priorPos:  make(map[string]map[string][3]float64),
		corpus:    corpus,
		angles:    angles,
		pending:   make([]pendingDial, 0),
		ui:        ui,
		binui:     binui,
	}
}

func newPhysics(config pmanifold.Config) (*pmanifold.Solver, error) {
	var physics *pmanifold.Solver

	err := compute.WithMetalInit(func() error {
		created, err := pmanifold.NewSolver(config)
		if err != nil {
			return err
		}
		physics = created
		return nil
	})

	if err != nil {
		return nil, err
	}

	return physics, nil
}

func (solver *Solver) ParticleCount() int {
	if solver == nil {
		return 0
	}
	return len(solver.oscillators)
}

func (solver *Solver) Name() string {
	return "manifold"
}

func (solver *Solver) Update(thesis *types.Thesis) error {
	oscillators := make([]pmanifold.Oscillator, 0)
	perSymbol := make(map[string][]pmanifold.Oscillator)

	var aggregateHawkes HawkesSignal

	symbolNames := make([]string, 0)

	thesis.Symbols.Range(func(key, value any) bool {
		symbolName, nameOK := key.(string)
		symbol, ok := value.(*types.Symbol)

		if !nameOK || symbolName == "" || !ok || symbol == nil {
			return true
		}

		symbolNames = append(symbolNames, symbolName)
		return true
	})

	if len(symbolNames) == 0 {
		return nil
	}

	slices.Sort(symbolNames)
	totalSymbols := len(symbolNames)
	symbolCount := 0

	for symbolIndex, symbolName := range symbolNames {
		rawSymbol, ok := thesis.Symbols.Load(symbolName)

		if !ok || rawSymbol == nil {
			continue
		}

		symbol := rawSymbol.(*types.Symbol)
		hawkes := extractSymbolHawkes(symbol)
		aggregateHawkes.Reflexivity += hawkes.Reflexivity
		aggregateHawkes.AvgTradeSize += hawkes.AvgTradeSize
		symbolCount++

		mapped, err := solver.bookOscillators(
			symbolName,
			symbolIndex,
			totalSymbols,
			thesis.At,
			hawkes,
		)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"manifold: failed to book oscillators for "+symbolName,
				err,
			))
			continue
		}

		if len(mapped) == 0 {
			continue
		}

		perSymbol[symbolName] = mapped
		oscillators = append(oscillators, mapped...)
	}

	if len(oscillators) == 0 {
		return nil
	}

	maxParticles := int(solver.config.MaxParticles)

	if maxParticles > 0 && len(oscillators) > maxParticles {
		oscillators = oscillators[:maxParticles]
	}

	if symbolCount > 1 {
		aggregateHawkes.Reflexivity /= float64(symbolCount)
		aggregateHawkes.AvgTradeSize /= float64(symbolCount)
	}

	if err := solver.physics.SetOscillators(oscillators); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf(
				"failed to load %d manifold oscillators across %d symbols: %v",
				len(oscillators), len(perSymbol), err,
			),
			err,
		))
	}

	solver.oscillators = oscillators

	// Advance manifold with top-down GPE controls modulated by Hawkes reflexivity
	return solver.Step(thesis, thesis.At, perSymbol, aggregateHawkes)
}

/*
bookOscillators projects every visible L3 order on one pair into oscillators
that belong to that pair's carrier, factoring in volume scaling, prior position velocity,
and Hawkes excitation frequency boosts.
*/
func (solver *Solver) bookOscillators(
	symbol string,
	symbolIndex int,
	totalSymbols int,
	at time.Time,
	hawkes HawkesSignal,
) ([]pmanifold.Oscillator, error) {
	var orders []struct {
		id        string
		side      mgrbook.BookDirection
		price     float64
		quantity  float64
		timestamp time.Time
	}
	var midPrice float64

	solver.api.Book(symbol, func(managed *mgrbook.Book) {
		if managed == nil {
			return
		}

		midPrice = managed.Midpoint().Float64()

		for _, level := range managed.Bids.Levels {
			for _, order := range level.Queue() {
				if order == nil || order.Quantity == nil || order.LimitPrice == nil {
					continue
				}

				quantity := order.Quantity.Float64()
				price := order.LimitPrice.Float64()

				if quantity <= 0 || price <= 0 {
					continue
				}

				orders = append(orders, struct {
					id        string
					side      mgrbook.BookDirection
					price     float64
					quantity  float64
					timestamp time.Time
				}{order.ID, mgrbook.Bid, price, quantity, order.Timestamp})
			}
		}

		for _, level := range managed.Asks.Levels {
			for _, order := range level.Queue() {
				if order == nil || order.Quantity == nil || order.LimitPrice == nil {
					continue
				}

				quantity := order.Quantity.Float64()
				price := order.LimitPrice.Float64()

				if quantity <= 0 || price <= 0 {
					continue
				}

				orders = append(orders, struct {
					id        string
					side      mgrbook.BookDirection
					price     float64
					quantity  float64
					timestamp time.Time
				}{order.ID, mgrbook.Ask, price, quantity, order.Timestamp})
			}
		}
	})

	if len(orders) == 0 || midPrice <= 0 {
		return nil, nil
	}

	accumulated, ok := solver.scales[symbol]
	if !ok {
		accumulated = adaptive.NewAccumulator()
		solver.scales[symbol] = accumulated
	}

	logSizes := make([]float64, 0, len(orders))
	var last adaptive.AccumulatorOutput
	totalVol := 0.0

	for _, order := range orders {
		deviation := math.Log(order.price) - math.Log(midPrice)
		measured, err := accumulated.Measure(deviation * deviation)
		if err != nil {
			return nil, err
		}

		last = measured
		totalVol += order.quantity
		logSizes = append(logSizes, math.Log(order.quantity))
	}

	if last.Count == 0 {
		return nil, nil
	}

	scale := math.Sqrt(last.Value / float64(last.Count))
	solver.converged[symbol] = scale
	meanVol := totalVol / float64(len(orders))

	ageOrder := make([]int, len(orders))
	queueOrder := make([]int, len(orders))

	for index := range orders {
		ageOrder[index] = index
		queueOrder[index] = index
	}

	sort.Slice(ageOrder, func(left, right int) bool {
		if !orders[ageOrder[left]].timestamp.Equal(orders[ageOrder[right]].timestamp) {
			return orders[ageOrder[left]].timestamp.Before(orders[ageOrder[right]].timestamp)
		}
		return orders[ageOrder[left]].id < orders[ageOrder[right]].id
	})

	sort.Slice(queueOrder, func(left, right int) bool {
		first := orders[queueOrder[left]]
		second := orders[queueOrder[right]]

		if first.side != second.side {
			return first.side < second.side
		}

		if first.price != second.price {
			if first.side == mgrbook.Bid {
				return first.price > second.price
			}
			return first.price < second.price
		}

		if !first.timestamp.Equal(second.timestamp) {
			return first.timestamp.Before(second.timestamp)
		}

		return first.id < second.id
	})

	ageRank := make([]int, len(orders))
	queueRank := make([]int, len(orders))
	sideCount := map[mgrbook.BookDirection]int{}

	for rank, index := range ageOrder {
		ageRank[index] = rank
	}

	for _, index := range queueOrder {
		queueRank[index] = sideCount[orders[index].side]
		sideCount[orders[index].side]++
	}

	omegaMin := solver.config.GateWidthMin()
	omegaMax := solver.config.GateWidthMax()
	omegaCentre := (omegaMax + omegaMin) / 2
	omegaHalf := (omegaMax - omegaMin) / 2

	priors := solver.priorPos[symbol]
	if priors == nil {
		priors = make(map[string][3]float64)
	}

	next := make(map[string][3]float64, len(orders))
	oscillators := make([]pmanifold.Oscillator, 0, len(orders))
	dt := solver.config.DeltaT
	maxVelocity := solver.config.MinGasCellSpacing() / solver.config.DeltaT

	for index, order := range orders {
		signedLog := math.Log(order.price) - math.Log(midPrice)

		// 1. Position X: Relative Price Depth (safely centered inside open boundary interval)
		posX := 0.5 * solver.config.DomainX
		omega := omegaCentre

		if scale > 0 {
			normalizedDist := math.Tanh(signedLog / scale)
			posX = (0.5 + 0.49*normalizedDist) * solver.config.DomainX
			// Hawkes excitation speeds up oscillator frequency tempo within lattice bounds
			hawkesBoost := (hawkes.LambdaBuy + hawkes.LambdaSell) * 0.05
			rawOmega := omegaCentre + (omegaHalf+hawkesBoost)*normalizedDist
			omega = math.Max(omegaMin, math.Min(omegaMax, rawOmega))
		}

		// 2. Position Y: Book Side Channel (distributed across symbol lanes)
		if totalSymbols <= 0 {
			totalSymbols = 1
		}

		laneSpan := solver.config.DomainY / float64(totalSymbols)
		laneCenter := (float64(symbolIndex) + 0.5) * laneSpan
		posY := laneCenter - 0.25*laneSpan

		if order.side == mgrbook.Ask {
			posY = laneCenter + 0.25*laneSpan
		}

		// 3. Position Z: Queue Depth / Age Rank (cell-centered to prevent torus wrap collisions)
		posZ := 0.5 * solver.config.DomainZ

		if len(orders) > 0 {
			posZ = (float64(ageRank[index]) + 0.5) / float64(len(orders)) * solver.config.DomainZ
		}

		// 4. Amplitude / Mass: Log-normalized volume scaling to avoid explosive blast waves
		normVolume := 1.0 + math.Log1p(order.quantity/math.Max(meanVol, 1e-8))
		amplitude := math.Sqrt(normVolume)

		// 5. Velocity derived from prior position, bounded by CFL acoustic speed
		var velX, velY, velZ float64

		if prior, exists := priors[order.id]; exists && dt > 0 {
			rawVelX := (posX - prior[0]) / dt
			rawVelY := (posY - prior[1]) / dt
			rawVelZ := (posZ - prior[2]) / dt

			velX = math.Max(-maxVelocity, math.Min(maxVelocity, rawVelX))
			velY = math.Max(-maxVelocity, math.Min(maxVelocity, rawVelY))
			velZ = math.Max(-maxVelocity, math.Min(maxVelocity, rawVelZ))
		}

		next[order.id] = [3]float64{posX, posY, posZ}

		// 6. Phase: FIFO Queue Progress + Side Opposition
		sideTotal := sideCount[order.side]
		progress := 0.0

		if sideTotal > 0 {
			progress = float64(queueRank[index]) / float64(sideTotal)
		}

		phase := math.Pi * progress

		if order.side == mgrbook.Ask {
			phase += math.Pi
		}

		// 7. Metabolic Heat: Proportional to volume and excitation flux in thermodynamic equilibrium
		excitationFlux := 1.0 + (hawkes.LambdaBuy+hawkes.LambdaSell)*dt
		heat := normVolume * solver.config.CV * excitationFlux

		oscillator := pmanifold.Oscillator{
			Phase:     math.Mod(phase, 2*math.Pi),
			Omega:     omega,
			Amplitude: amplitude,
			PosX:      posX,
			PosY:      posY,
			PosZ:      posZ,
			Heat:      heat,
			VelX:      velX,
			VelY:      velY,
			VelZ:      velZ,
		}

		// No oscillator's math is blindly trusted: every field is validated
		// before it reaches the GPU. A zero heat or amplitude is divided by
		// downstream, and any non-finite field poisons the mode integrators
		// for every carrier sharing the lattice.
		if degenerateOscillator(oscillator) {
			errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf(
					"manifold: dropping degenerate oscillator for %s order %s: %+v",
					symbol, order.id, oscillator,
				),
				nil,
			))

			delete(next, order.id)
			continue
		}

		oscillators = append(oscillators, oscillator)
	}

	solver.priorPos[symbol] = next

	return oscillators, nil
}

func (solver *Solver) Step(
	thesis *types.Thesis,
	at time.Time,
	perSymbol map[string][]pmanifold.Oscillator,
	hawkes HawkesSignal,
) error {
	controls := solver.config.RuntimeControls()
	controls.DeltaT = solver.config.DeltaT

	// Modulate top-down GPE controls using Hawkes reflexivity η
	controls.TopdownPhaseScale = math.Min(1.0, math.Max(0.0, hawkes.Reflexivity))
	controls.TopdownEnergyScale = math.Min(2.0, math.Max(0.0, hawkes.Reflexivity*hawkes.Reflexivity))

	if err := solver.physics.SetControls(controls); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"manifold: failed to apply runtime controls",
			err,
		))
	}

	reading, err := solver.physics.Step()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("failed to advance manifold: %v", err),
			err,
		))
	}

	solver.reading = reading
	thesis.StoreManifold(reading)

	solver.publishPhase(thesis, at, perSymbol)

	if len(solver.oscillators) > 0 {
		oscillators, readErr := solver.physics.ReadOscillators(len(solver.oscillators))
		if readErr != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"failed to read manifold oscillators: "+readErr.Error(),
				readErr,
			))
		}

		solver.oscillators = oscillators
	}

	if err := solver.publishDomain(); err != nil {
		return err
	}

	return nil
}

func (solver *Solver) publishDomain() error {
	if solver.binui == nil {
		return nil
	}

	vol, err := solver.physics.ReadVolumetricFields(16)
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf(
				"failed to read manifold volumetric fields with %d resident oscillators: %v",
				solver.ParticleCount(), err,
			),
			err,
		))
	}

	utils.PublishFluid(
		solver.binui, types.FluidFieldsChannel,
		datura.NewMap("fields", projectFields(vol)),
	)
	utils.PublishFluid(
		solver.binui, types.FluidParticlesChannel,
		datura.NewMap("particles", oscillatorsToParticles(solver.config, solver.oscillators)),
	)

	return nil
}

func (solver *Solver) publishPhase(
	thesis *types.Thesis,
	at time.Time,
	perSymbol map[string][]pmanifold.Oscillator,
) {
	reading := solver.stampPhase(thesis, at, perSymbol)

	utils.Publish(
		solver.ui,
		datura.NewMap(
			"manifold",
			[]datura.Map[any]{solver.phaseRow(at, reading)},
		),
	)

	if solver.binui != nil {
		utils.PublishFluid(
			solver.binui,
			types.FluidPhaseChannel,
			solver.phaseRow(at, reading),
		)
	}
}

func (solver *Solver) phaseRow(
	at time.Time,
	reading types.PhaseReading,
) datura.Map[any] {
	kuramoto := kuramotoOrderParameter(solver.oscillators)

	row := datura.NewMap(
		"source", "manifold",
		"at", at.Format(time.RFC3339),
		"phaseReady", reading.Ready,
		"phaseReason", reading.Reason,
		"wave", oscillatorWave(solver.oscillators),
		"hydrodynamics", datura.NewMap(
			"pressureGradNorm", solver.reading.PressureGradNorm,
			"divergence", solver.reading.Divergence,
			"coherenceMag2", solver.reading.CoherenceMag2,
			"guidanceSpeed", solver.reading.GuidanceSpeed,
			"viscosityProxy", solver.reading.ViscosityProxy,
			"kuramotoR", kuramoto.R,
			"kuramotoPsi", kuramoto.Psi,
		),
	)

	if reading.Ready {
		row["phaseScan"] = reading.Responses
	}

	return row
}

type KuramotoSync struct {
	R   float64 `json:"r"`
	Psi float64 `json:"psi"`
}

func kuramotoOrderParameter(oscillators []pmanifold.Oscillator) KuramotoSync {
	if len(oscillators) == 0 {
		return KuramotoSync{}
	}

	var sumCos, sumSin float64

	for _, osc := range oscillators {
		sumCos += math.Cos(osc.Phase)
		sumSin += math.Sin(osc.Phase)
	}

	count := float64(len(oscillators))
	meanCos := sumCos / count
	meanSin := sumSin / count

	r := math.Sqrt(meanCos*meanCos + meanSin*meanSin)
	psi := math.Atan2(meanSin, meanCos)

	return KuramotoSync{
		R:   r,
		Psi: psi,
	}
}

func (solver *Solver) Close() error {
	if solver == nil {
		return nil
	}

	if !solver.closing.CompareAndSwap(false, true) {
		return nil
	}

	if solver.physics == nil {
		return nil
	}

	solver.physics.Close()
	solver.physics = nil

	return nil
}

func (solver *Solver) scale(symbol string) (float64, bool) {
	if solver == nil {
		return 0, false
	}

	value, ok := solver.converged[symbol]
	return value, ok && value > 0
}

/*
finitePositive admits a metric sample into the manifold only when it is a
strictly positive, finite real number. A non-finite or non-positive fit is a
degenerate measurement: it is excluded at this boundary rather than allowed to
poison the GPU state it feeds.
*/
func finitePositive(value float64) bool {
	return value > 0 && !math.IsInf(value, 0)
}

/*
degenerateOscillator reports whether any field of one oscillator would poison
the GPU lattice. Positions, tempo, and velocity only need to be finite; heat
and amplitude are divided by downstream, so they must be strictly positive.
*/
func degenerateOscillator(oscillator pmanifold.Oscillator) bool {
	fields := []float64{
		oscillator.Phase,
		oscillator.Omega,
		oscillator.PosX,
		oscillator.PosY,
		oscillator.PosZ,
		oscillator.VelX,
		oscillator.VelY,
		oscillator.VelZ,
	}

	for _, field := range fields {
		if math.IsNaN(field) || math.IsInf(field, 0) {
			return true
		}
	}

	return !finitePositive(oscillator.Heat) || !finitePositive(oscillator.Amplitude) || !finitePositive(oscillator.Omega)
}

func extractSymbolHawkes(symbol *types.Symbol) HawkesSignal {
	signal := HawkesSignal{
		LambdaBuy:    1.0,
		LambdaSell:   1.0,
		LambdaCancel: 0.5,
		Reflexivity:  0.2,
		AvgTradeSize: 1.0,
	}

	if symbol == nil || symbol.Latest == nil {
		return signal
	}

	raw, ok := symbol.Latest.Load(string(types.SourceHawkes))
	if !ok || raw == nil {
		return signal
	}

	measurement, ok := raw.(*types.Measurement)
	if !ok || measurement == nil || measurement.Metrics == nil {
		return signal
	}

	buyIntensityKey := types.MetricKey(types.MetricConditionalIntensity, types.SideBuy)

	if sample, exists := measurement.Metrics[buyIntensityKey]; exists && finitePositive(sample.Raw) {
		signal.LambdaBuy = sample.Raw
	} else {
		arrBuyKey := types.MetricKey(types.MetricArrivalRate, types.SideBuy)

		if arrSample, exists := measurement.Metrics[arrBuyKey]; exists && finitePositive(arrSample.Raw) {
			signal.LambdaBuy = arrSample.Raw
		}
	}

	sellIntensityKey := types.MetricKey(types.MetricConditionalIntensity, types.SideSell)

	if sample, exists := measurement.Metrics[sellIntensityKey]; exists && finitePositive(sample.Raw) {
		signal.LambdaSell = sample.Raw
	} else {
		arrSellKey := types.MetricKey(types.MetricArrivalRate, types.SideSell)

		if arrSample, exists := measurement.Metrics[arrSellKey]; exists && finitePositive(arrSample.Raw) {
			signal.LambdaSell = arrSample.Raw
		}
	}

	spectralRadiusKey := types.MetricKey(types.MetricSpectralRadius, types.SideNone)
	if sample, exists := measurement.Metrics[spectralRadiusKey]; exists && sample.Raw >= 0 {
		signal.Reflexivity = math.Min(1.0, sample.Raw)
	}

	return signal
}

type OrderVector struct {
	X float32 `json:"X"`
	Y float32 `json:"Y"`
	Z float32 `json:"Z"`
}

type OrderParticle struct {
	Position OrderVector `json:"Position"`
	Velocity OrderVector `json:"Velocity"`
	Mass     float32     `json:"Mass"`
	Heat     float32     `json:"Heat"`
	Energy   float32     `json:"Energy"`
}

type OrderOscillator struct {
	Phase     float32 `json:"Phase"`
	Omega     float32 `json:"Omega"`
	Amplitude float32 `json:"Amplitude"`
	Real      float32 `json:"Real"`
	Imaginary float32 `json:"Imaginary"`
}

type OrderNode struct {
	Position   OrderVector     `json:"Position"`
	Velocity   OrderVector     `json:"Velocity"`
	Mass       float32         `json:"Mass"`
	Heat       float32         `json:"Heat"`
	Energy     float32         `json:"Energy"`
	Phase      float32         `json:"Phase"`
	Omega      float32         `json:"Omega"`
	Amplitude  float32         `json:"Amplitude"`
	Particle   OrderParticle   `json:"particle"`
	Oscillator OrderOscillator `json:"oscillator"`
}

type ManifoldGrid struct {
	X       int     `json:"x"`
	Y       int     `json:"y"`
	Z       int     `json:"z"`
	Spacing float32 `json:"spacing"`
}

type ManifoldFields struct {
	Grid           ManifoldGrid `json:"Grid"`
	Density        []float32    `json:"Density"`
	Momentum       []float32    `json:"Momentum"`
	InternalEnergy []float32    `json:"InternalEnergy"`
	WaveReal       []float32    `json:"WaveReal"`
	WaveImaginary  []float32    `json:"WaveImaginary"`
}

func oscillatorsToParticles(
	config pmanifold.Config,
	oscillators []pmanifold.Oscillator,
) []OrderNode {
	particles := make([]OrderNode, len(oscillators))
	domainX := float32(config.DomainX)
	domainY := float32(config.DomainY)
	domainZ := float32(config.DomainZ)

	if domainX <= 0 {
		domainX = 1
	}
	if domainY <= 0 {
		domainY = 1
	}
	if domainZ <= 0 {
		domainZ = 1
	}

	for index, oscillator := range oscillators {
		normPos := OrderVector{
			X: float32(oscillator.PosX) / domainX,
			Y: float32(oscillator.PosY) / domainY,
			Z: float32(oscillator.PosZ) / domainZ,
		}
		normVel := OrderVector{
			X: float32(oscillator.VelX) / domainX,
			Y: float32(oscillator.VelY) / domainY,
			Z: float32(oscillator.VelZ) / domainZ,
		}
		mass := float32(oscillator.Amplitude)
		heat := float32(oscillator.Heat)
		energy := float32(oscillator.Amplitude * oscillator.Amplitude)
		phase := float32(oscillator.Phase)
		omega := float32(oscillator.Omega)
		amp := float32(oscillator.Amplitude)
		real := float32(oscillator.Amplitude * math.Cos(oscillator.Phase))
		imag := float32(oscillator.Amplitude * math.Sin(oscillator.Phase))

		particle := OrderParticle{
			Position: normPos,
			Velocity: normVel,
			Mass:     mass,
			Heat:     heat,
			Energy:   energy,
		}
		osc := OrderOscillator{
			Phase:     phase,
			Omega:     omega,
			Amplitude: amp,
			Real:      real,
			Imaginary: imag,
		}

		particles[index] = OrderNode{
			Position:   normPos,
			Velocity:   normVel,
			Mass:       mass,
			Heat:       heat,
			Energy:     energy,
			Phase:      phase,
			Omega:      omega,
			Amplitude:  amp,
			Particle:   particle,
			Oscillator: osc,
		}
	}

	return particles
}

func projectFields(
	vol pmanifold.VolumetricFields,
) ManifoldFields {
	spacing := 1.0 / float32(vol.GridX)
	if vol.GridX <= 0 {
		spacing = 1.0 / 64.0
	}

	cells := len(vol.WaveReal)
	waveMag := make([]float32, cells)
	waveZero := make([]float32, cells)

	for cell := range cells {
		re := vol.WaveReal[cell]
		im := vol.WaveImaginary[cell]
		waveMag[cell] = re*re + im*im
	}

	return ManifoldFields{
		Grid: ManifoldGrid{
			X:       vol.GridX,
			Y:       vol.GridY,
			Z:       vol.GridZ,
			Spacing: spacing,
		},
		Density:        vol.Density,
		Momentum:       vol.Momentum,
		InternalEnergy: vol.InternalEnergy,
		WaveReal:       waveMag,
		WaveImaginary:  waveZero,
	}
}
