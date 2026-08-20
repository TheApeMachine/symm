package manifold

import (
	"fmt"
	"math"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	mgrbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/geometry"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/signal/compute"
	"github.com/theapemachine/symm/telemetry"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

/*
phaseLatticeWidth is the pair-carrier census and the ω-bin count of the
universe phase dial. Each subscribed symbol occupies one carrier. Level 3
orders on that pair are oscillators in that carrier's collection.
*/
const phaseLatticeWidth uint32 = 256

/*
oscillatorPoolCapacity reserves room for every visible Level 3 order loaded in
one pass. The universe is whatever Kraken lists online in the quote currency
at boot; the symbol registry budget is 1024, and live books show ~68 visible
orders per symbol, so the pool is their product. It is allocated once —
headroom costs memory, not per-step time — and a deeper universe fails loudly
here rather than truncating the physics.
*/
const oscillatorPoolCapacity uint32 = 1024 * 68

/*
Solver owns one resident Sensorium domain for the complete market universe.
Symbols contribute observations to the same gas and wave fields; they are not
split into independent simulations that cannot interfere.
*/
type Solver struct {
	err         error
	api         websocket.BookSource
	config      pmanifold.Config
	physics     *pmanifold.Solver
	oscillators []pmanifold.Oscillator
	reading     pmanifold.Reading
	recorder    *audit.Recorder
	universe    []string
	scales      map[string]*adaptive.Accumulator
	converged   map[string]float64
	priorPos    map[string]map[string][3]float64
	priorAt     time.Time
	corpus      *geometry.Corpus[types.PhaseOutcome]
	angles      []float64
	pending     []pendingDial
	waiting     atomic.Bool
	ui          *transport.MapReduce[[]byte]
	binui       *transport.MapReduce[types.FluidFrame]
	semaphore   chan struct{}
	stopping    chan struct{}
	stopped     chan struct{}
	closing     atomic.Bool
	running     atomic.Bool
	settling    atomic.Bool
	requestMu   sync.Mutex
	requested   uint64
	completed   uint64
	latest      manifoldRequest
	stepped     bool
	driveEta    float64
	driveBeta   float64
}

/*
manifoldCut keeps the oscillators attributed to one symbol while every cut in
the market update relaxes inside the same resident domain.
*/
type manifoldCut struct {
	symbol      string
	carrier     uint32
	oscillators []pmanifold.Oscillator
}

/*
manifoldRequest is the latest state the single manifold owner has been asked to
relax. It is a coalescing slot, not an inbox: bursts replace an obsolete request
while the measurement queues retain every unconsumed observation.
*/
type manifoldRequest struct {
	generation uint64
	thesis     *types.Thesis
	at         time.Time
}

/*
NewSolver creates the single shared Metal domain and a spectral corpus bounded
by the same explicit event-history capacity as the live market feed.
*/
func NewSolver(
	api *websocket.API,
	ui *transport.MapReduce[[]byte],
	binui *transport.MapReduce[types.FluidFrame],
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
		oscillatorPoolCapacity,
	)
	errnie.Error(err)
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

	solver := &Solver{
		api:       bookSource,
		config:    config,
		physics:   physics,
		recorder:  recorder,
		scales:    make(map[string]*adaptive.Accumulator),
		converged: make(map[string]float64),
		priorPos:  make(map[string]map[string][3]float64),
		corpus:    corpus,
		angles:    angles,
		ui:        ui,
		binui:     binui,
		semaphore: make(chan struct{}, 1),
		stopping:  make(chan struct{}),
		stopped:   make(chan struct{}),
	}
	return solver
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

/*
Settling reports whether a loaded iteration's single physics step is still
running. Replay uses this so the next captured frame cannot arrive before the
current field has settled.
*/
func (solver *Solver) Settling() bool {
	return solver != nil && solver.settling.Load()
}

func (solver *Solver) Name() string {
	return "manifold"
}

func (solver *Solver) Error() error { return solver.err }

/*
Update walks every busy book's L3 orders into the resident oscillator
population and wakes settlement. Calls received while that worker is stepping
leave their measurements queued.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	if solver == nil || thesis == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"manifold: solver and thesis required",
			nil,
		))
	}

	if solver.physics == nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"manifold: physics domain is not initialized",
			nil,
		))
	}

	if solver.api == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"manifold: authoritative order book source required",
			nil,
		))
	}

	solver.requestMu.Lock()

	if solver.closing.Load() {
		solver.requestMu.Unlock()

		return errnie.Error(errnie.Err(
			errnie.Validation,
			"manifold: solver is closing",
			nil,
		))
	}

	solver.requested++
	solver.latest = manifoldRequest{
		generation: solver.requested,
		thesis:     thesis,
		at:         thesis.At,
	}
	wake := solver.settling.CompareAndSwap(false, true)
	solver.requestMu.Unlock()

	if wake {
		select {
		case solver.semaphore <- struct{}{}:
		default:
		}
	}

	return nil
}

func (solver *Solver) Run() error {
	if solver.closing.Load() {
		return nil
	}

	if !solver.running.CompareAndSwap(false, true) {
		solver.err = errnie.Error(errnie.Err(
			errnie.Conflict,
			"manifold: solver is already running",
			nil,
		))

		return solver.err
	}

	defer close(solver.stopped)
	defer solver.settling.Store(false)

	for solver.err == nil {
		select {
		case <-solver.semaphore:
			if err := solver.drainRequests(); err != nil {
				solver.err = err
			}
		case <-solver.stopping:
			return nil
		}
	}

	return solver.err
}

/*
drainRequests keeps the resident manifold owner active until it has consumed
its latest requested generation. Intermediate requests may coalesce, but the
final request in a burst cannot be stranded after settling becomes false.
*/
func (solver *Solver) drainRequests() error {
	for {
		request, available := solver.nextRequest()

		if !available {
			return nil
		}

		cuts, err := solver.load(request.thesis, request.at)

		if err == nil && len(cuts) > 0 {
			err = solver.Step(request.thesis, request.at, cuts)
		}

		if err != nil {
			return errnie.Error(err)
		}

		solver.completeRequest(request.generation)
	}
}

func (solver *Solver) nextRequest() (manifoldRequest, bool) {
	solver.requestMu.Lock()
	defer solver.requestMu.Unlock()

	if solver.completed >= solver.requested {
		solver.settling.Store(false)
		return manifoldRequest{}, false
	}

	return solver.latest, true
}

func (solver *Solver) completeRequest(generation uint64) {
	solver.requestMu.Lock()

	if generation > solver.completed {
		solver.completed = generation
	}

	solver.requestMu.Unlock()
}

func (solver *Solver) load(thesis *types.Thesis, at time.Time) ([]manifoldCut, error) {
	if solver.physics == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"manifold: physics domain is not initialized",
			nil,
		))
	}

	if solver.api == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"manifold: authoritative order book source required",
			nil,
		))
	}

	solver.waiting.Store(false)
	type drive struct {
		buy, sell, eta, beta float64
		hasHawkes            bool
	}
	drives := make(map[string]drive)

	thesis.Symbols.Range(func(key, value any) bool {
		symbolName, nameOK := key.(string)
		symbol, ok := value.(*types.Symbol)

		if !nameOK || symbolName == "" || !ok || symbol == nil {
			return true
		}

		if symbol.Status == types.BUSY {
			drives[symbolName] = drive{}
		}

		for measurement := range symbol.MarketMeasurements("manifold") {
			if string(measurement.Source) != string(types.SourceHawkes) {
				continue
			}

			entry := drives[symbolName]
			entry.hasHawkes = true
			buySample := measurement.Metrics[types.MetricKey(
				types.MetricExcitationAmplitude, types.SideBuyToBuy,
			)]
			sellSample := measurement.Metrics[types.MetricKey(
				types.MetricExcitationAmplitude, types.SideSellToSell,
			)]

			if buySample.Normalized != nil {
				entry.buy = *buySample.Normalized
			}

			if sellSample.Normalized != nil {
				entry.sell = *sellSample.Normalized
			}

			entry.eta = measurement.Metrics[types.MetricKey(
				types.MetricSpectralRadius, types.SideNone,
			)].Raw
			entry.beta = measurement.Metrics[types.MetricKey(
				types.MetricDecayRate, types.SideNone,
			)].Raw
			drives[symbolName] = entry
		}

		return true
	})
	symbolNames := make([]string, 0, len(drives))

	for symbolName := range drives {
		symbolNames = append(symbolNames, symbolName)
	}

	sort.Strings(symbolNames)
	cuts := make([]manifoldCut, 0, len(symbolNames))
	oscillators := make([]pmanifold.Oscillator, 0)
	etaMass := 0.0
	betaMass := 0.0
	driveMass := 0.0

	for _, symbolName := range symbolNames {
		if _, known := universeIndex(solver.universe, symbolName); !known {
			solver.universe = sortedUniverse(append(solver.universe, symbolName))
		}

		if uint32(len(solver.universe)) > phaseLatticeWidth {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"manifold: symbol-pair carriers exceed the 256-slot lattice",
				nil,
			))
		}

		carrier, known := universeIndex(solver.universe, symbolName)

		if !known {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"manifold: symbol "+symbolName+" is outside the pair lattice",
				nil,
			))
		}

		drive := drives[symbolName]
		mapped, err := solver.bookOscillators(
			symbolName, drive.buy, drive.sell, at,
		)

		if err != nil {
			return nil, err
		}

		if len(mapped) == 0 {
			solver.waiting.Store(true)
			continue
		}

		oscillators = append(oscillators, mapped...)
		cuts = append(cuts, manifoldCut{
			symbol:      symbolName,
			carrier:     carrier,
			oscillators: mapped,
		})

		if drive.hasHawkes {
			mass := float64(len(mapped))
			etaMass += drive.eta * mass
			betaMass += drive.beta * mass
			driveMass += mass
		}
	}

	if len(oscillators) == 0 {
		solver.waiting.Store(true)
		return nil, nil
	}

	if err := solver.physics.ResetDeposits(); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"manifold: failed to reset deposits",
			err,
		))
	}

	for _, cut := range cuts {
		drive := drives[cut.symbol]

		if err := solver.depositPair(cut.carrier, drive.buy, drive.sell); err != nil {
			return nil, err
		}
	}

	if err := solver.physics.SetOscillators(oscillators); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf(
				"failed to load %d manifold oscillators across %d symbols: %v",
				len(oscillators), len(cuts), err,
			),
			err,
		))
	}

	solver.oscillators = oscillators
	solver.priorAt = at

	if driveMass > 0 {
		solver.driveEta = etaMass / driveMass
		solver.driveBeta = betaMass / driveMass
	}

	return cuts, nil
}

/*
bookOscillators projects every visible L3 order on one pair into oscillators
that belong to that pair's carrier.
*/
func (solver *Solver) bookOscillators(
	symbol string,
	buyExcitation float64,
	sellExcitation float64,
	at time.Time,
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

	ages := make([]float64, len(orders))
	logSizes := make([]float64, 0, len(orders))
	var last adaptive.AccumulatorOutput

	for index, order := range orders {
		deviation := math.Log(order.price) - math.Log(midPrice)
		measured, err := accumulated.Measure(deviation * deviation)

		if err != nil {
			return nil, err
		}

		last = measured
		ages[index] = at.Sub(order.timestamp).Seconds()
		logSizes = append(logSizes, math.Log(order.quantity))
	}

	if last.Count == 0 {
		return nil, nil
	}

	scale := math.Sqrt(last.Value / float64(last.Count))
	solver.converged[symbol] = scale
	sizeMin := logSizes[0]
	sizeMax := logSizes[0]

	for _, logSize := range logSizes {
		sizeMin = min(sizeMin, logSize)
		sizeMax = max(sizeMax, logSize)
	}

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
			return first.side == mgrbook.Bid
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
	deltaT := 0.0

	if !solver.priorAt.IsZero() && at.After(solver.priorAt) {
		deltaT = at.Sub(solver.priorAt).Seconds()
	}

	prior := solver.priorPos[symbol]

	if prior == nil {
		prior = make(map[string][3]float64)
	}

	next := make(map[string][3]float64, len(orders))
	oscillators := make([]pmanifold.Oscillator, 0, len(orders))

	for index, order := range orders {
		energy := 1.0 + buyExcitation

		if order.side == mgrbook.Ask {
			energy = 1.0 + sellExcitation
		}

		if energy <= 0 {
			continue
		}

		signedLog := math.Log(order.price) - math.Log(midPrice)
		omega := omegaCentre
		posX := 0.5 * solver.config.DomainX

		if scale > 0 {
			omega = omegaCentre + omegaHalf*math.Tanh(signedLog/scale)
			posX = (0.5 + 0.5*math.Tanh(signedLog/scale)) * solver.config.DomainX
		}
		posY := 0.5 * solver.config.DomainY

		if sizeMax > sizeMin {
			posY = (math.Log(order.quantity) - sizeMin) / (sizeMax - sizeMin) * solver.config.DomainY
		}

		posZ := 0.5 * solver.config.DomainZ

		if len(orders) > 1 {
			posZ = float64(ageRank[index]) / float64(len(orders)-1) * solver.config.DomainZ
		}

		velX, velY, velZ := 0.0, 0.0, 0.0

		if previous, seen := prior[order.id]; seen && deltaT > 0 {
			velX = (posX - previous[0]) / deltaT
			velY = (posY - previous[1]) / deltaT
			velZ = (posZ - previous[2]) / deltaT
		}

		next[order.id] = [3]float64{posX, posY, posZ}
		sideTotal := sideCount[order.side]
		progress := 0.0

		if sideTotal > 0 {
			progress = float64(queueRank[index]) / float64(sideTotal)
		}

		phase := math.Pi * progress

		if order.side == mgrbook.Ask {
			phase += math.Pi
		}

		oscillators = append(oscillators, pmanifold.Oscillator{
			Phase:     math.Mod(phase, 2*math.Pi),
			Omega:     omega,
			Amplitude: math.Sqrt(energy),
			PosX:      posX,
			PosY:      posY,
			PosZ:      posZ,
			Heat:      1.0 / 32.0,
			VelX:      velX,
			VelY:      velY,
			VelZ:      velZ,
		})
	}

	solver.priorPos[symbol] = next

	return oscillators, nil
}

func (solver *Solver) depositPair(carrier uint32, buy float64, sell float64) error {
	total := buy + sell

	if total <= 0 {
		return nil
	}

	maxX := float64(solver.config.GridX - 1)
	buyCell := uint32(math.Round(buy / total * maxX))
	sellCell := uint32(math.Round(sell / total * maxX))
	cellY := solver.config.GridY / 2
	cellZ := carrier % solver.config.GridZ
	buyRho := buy / total * solver.config.RhoMin
	sellRho := sell / total * solver.config.RhoMin

	if err := solver.physics.DepositCell(
		buyCell, cellY, cellZ,
		buyRho, 0, 0, 0, buyRho*solver.config.CV,
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"manifold: failed to deposit buy intensity",
			err,
		))
	}

	return solver.physics.DepositCell(
		sellCell, cellY, cellZ,
		sellRho, 0, 0, 0, sellRho*solver.config.CV,
	)
}

/*
WaitingForBook answers whether Manifold has explicitly deferred any symbol
until its authoritative Level 3 manager publishes another update.
*/
func (solver *Solver) WaitingForBook() bool {
	return solver.waiting.Load()
}

func (solver *Solver) Step(
	thesis *types.Thesis,
	at time.Time,
	cuts []manifoldCut,
) error {
	if solver.physics == nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"manifold: physics domain is not initialized",
			nil,
		))
	}

	controls := solver.config.RuntimeControls()
	controls.DeltaT = solver.config.DeltaT

	if solver.driveBeta > 0 {
		advective := solver.config.AdvectiveDeltaT(solver.driveBeta)

		if advective > 0 && advective < controls.DeltaT {
			controls.DeltaT = advective
		}

		controls.EnergyDecay = solver.driveBeta
		controls.MetabolicRate = 1 / controls.DeltaT
	}

	if solver.driveEta > 0 {
		controls.GInteraction = solver.config.GInteraction() * solver.driveEta
	}

	if err := solver.physics.SetControls(controls); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"manifold: failed to apply runtime controls",
			err,
		))
	}

	// One physics step per loaded iteration: throughput is dominated by new
	// order flow, so the field advances exactly once over each fresh batch
	// rather than relaxing between observations that already arrived.
	reading, err := solver.physics.Step()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("failed to advance manifold: %v", err),
			err,
		))
	}

	solver.stepped = true
	solver.reading = reading
	thesis.StoreManifold(reading)

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

	solver.publishPhase(thesis, at, cuts)

	return solver.publishDomain()
}

func (solver *Solver) publishDomain() error {
	if solver.binui == nil {
		return nil
	}

	particles := oscillatorsToParticles(solver.config, solver.oscillators)
	rows := make([]*wire.FluidParticleT, len(particles))

	for index, particle := range particles {
		rows[index] = &wire.FluidParticleT{
			Position:  fluidVectorWire(particle.Position),
			Velocity:  fluidVectorWire(particle.Velocity),
			Mass:      particle.Mass,
			Heat:      particle.Heat,
			Energy:    particle.Energy,
			Phase:     particle.Phase,
			Omega:     particle.Omega,
			Amplitude: particle.Amplitude,
		}
	}

	solver.binui.Push(types.FluidFrame{
		Channel: types.FluidParticlesChannel,
		Payload: telemetry.Encode(&wire.FrameT{
			Type:  wire.FrameFluidParticlesFrame,
			Value: &wire.FluidParticlesFrameT{Particles: rows},
		}),
	})

	return nil
}

func (solver *Solver) publishPhase(
	thesis *types.Thesis,
	at time.Time,
	cuts []manifoldCut,
) {
	reading := solver.stampPhase(thesis, at, cuts)

	if solver.binui != nil {
		solver.binui.Push(types.FluidFrame{
			Channel: types.FluidPhaseChannel,
			Payload: telemetry.Encode(&wire.FrameT{
				Type:  wire.FrameFluidPhaseFrame,
				Value: solver.phaseRow(at, reading),
			}),
		})
	}
}

/*
phaseRow is the universe sweep as the wire already consumes it: resident wave,
readiness, angular scan, and real-time hydrodynamic / Kuramoto diagnostics.
*/
func (solver *Solver) phaseRow(
	at time.Time,
	reading types.PhaseReading,
) *wire.FluidPhaseFrameT {
	kuramoto := kuramotoOrderParameter(solver.oscillators)
	wave := oscillatorWave(solver.oscillators)
	waveWire := make([]*wire.WaveModeT, len(wave))

	for index, mode := range wave {
		waveWire[index] = &wire.WaveModeT{
			Omega: mode.Omega, Real: mode.Real, Imaginary: mode.Imaginary,
		}
	}

	return &wire.FluidPhaseFrameT{
		Source:      "manifold",
		At:          at.UnixNano(),
		PhaseReady:  reading.Ready,
		PhaseReason: reading.Reason,
		Wave:        waveWire,
		Hydrodynamics: &wire.HydrodynamicsT{
			PressureGradNorm: solver.reading.PressureGradNorm,
			Divergence:       solver.reading.Divergence,
			CoherenceMag2:    solver.reading.CoherenceMag2,
			GuidanceSpeed:    solver.reading.GuidanceSpeed,
			ViscosityProxy:   solver.reading.ViscosityProxy,
			KuramotoR:        kuramoto.R,
			KuramotoPsi:      kuramoto.Psi,
		},
		PhaseScan: phaseResponsesWire(reading.Responses),
	}
}

func fluidVectorWire(vector OrderVector) *wire.FluidVectorT {
	return &wire.FluidVectorT{X: vector.X, Y: vector.Y, Z: vector.Z}
}

func phaseResponsesWire(responses []types.PhaseResponse) []*wire.PhaseResponseT {
	rows := make([]*wire.PhaseResponseT, len(responses))

	for index, response := range responses {
		rows[index] = &wire.PhaseResponseT{
			Angle:      response.Angle,
			Similarity: response.Similarity,
			ObservedAt: response.ObservedAt,
			Outcome: &wire.PhaseOutcomeT{
				Direction:   response.Outcome.Direction,
				ReturnValue: response.Outcome.Return,
				Horizon:     int64(response.Outcome.Horizon),
			},
		}
	}

	return rows
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

/*
Close releases the one resident domain and all accumulated observations.
*/
func (solver *Solver) Close() error {
	if solver == nil {
		return nil
	}

	if !solver.closing.CompareAndSwap(false, true) {
		return nil
	}

	for solver.settling.Load() {
		runtime.Gosched()
	}

	close(solver.stopping)

	if solver.running.Load() {
		<-solver.stopped
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
OrderVector is a 3D float32 vector in unit coordinates.
*/
type OrderVector struct {
	X float32 `json:"X"`
	Y float32 `json:"Y"`
	Z float32 `json:"Z"`
}

/*
OrderParticle holds the geometric state of an order in the physical domain.
*/
type OrderParticle struct {
	Position OrderVector `json:"Position"`
	Velocity OrderVector `json:"Velocity"`
	Mass     float32     `json:"Mass"`
	Heat     float32     `json:"Heat"`
	Energy   float32     `json:"Energy"`
}

/*
OrderOscillator holds the wave state of an order in the coherence layer.
*/
type OrderOscillator struct {
	Phase     float32 `json:"Phase"`
	Omega     float32 `json:"Omega"`
	Amplitude float32 `json:"Amplitude"`
	Real      float32 `json:"Real"`
	Imaginary float32 `json:"Imaginary"`
}

/*
OrderNode represents an order in the manifold possessing both a particle
(geometric) and an oscillator (wave) representation.
*/
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

/*
ManifoldGrid is the 3D cell layout of the domain.
*/
type ManifoldGrid struct {
	X       int     `json:"x"`
	Y       int     `json:"y"`
	Z       int     `json:"z"`
	Spacing float32 `json:"spacing"`
}

/*
ManifoldFields represents the physical fields evaluated over the grid.
*/
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

func torusIndex(position, domain float64, grid uint32) uint32 {
	if grid == 0 || domain <= 0 {
		return 0
	}

	index := int(math.Floor(position*float64(grid)/domain)) % int(grid)

	if index < 0 {
		index += int(grid)
	}

	return uint32(index)
}

func universeIndex(names []string, symbol string) (uint32, bool) {
	index := sort.SearchStrings(names, symbol)

	if index == len(names) || names[index] != symbol {
		return 0, false
	}

	return uint32(index), true
}

func sortedUniverse(names []string) []string {
	universe := append([]string(nil), names...)
	sort.Strings(universe)

	return universe
}
