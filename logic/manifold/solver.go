package manifold

import (
	"context"
	"fmt"
	nomagiqueruntime "github.com/theapemachine/symm/nomagique/runtime"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	types "github.com/theapemachine/symm/types"
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
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken/websocket"
	pmanifold "github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/telemetry"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
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
	ctx          context.Context
	err          error
	thesis       *types.Thesis
	api          websocket.BookSource
	config       Domain
	physics      *pmanifold.Manifold
	slabs        slabEncoder
	oscillators  []Oscillator
	reading      pmanifold.Reading
	recorder     *audit.Recorder
	universe     []string
	scales       map[string]*adaptive.Accumulator
	converged    map[string]float64
	drives       sync.Map
	priorPos     map[string]map[string][3]float64
	priorAt      time.Time
	corpus       *geometry.Corpus[types.PhaseOutcome]
	angles       []float64
	pending      []pendingDial
	waiting      atomic.Bool
	fluid        *nomagiqueruntime.Channel[types.FluidFrame]
	semaphore    chan struct{}
	stopping     chan struct{}
	stopped      chan struct{}
	closing      atomic.Bool
	running      atomic.Bool
	settling     atomic.Bool
	requested    atomic.Uint64
	completed    atomic.Uint64
	latest       atomic.Pointer[manifoldRequest]
	work         *nomagiqueruntime.Subscription[*nmtypes.Measurement]
	stepped      bool
	driveEta     float64
	driveBeta    float64
	stepInterval time.Duration
}

/*
manifoldDrive is one symbol's latest Hawkes excitation read, folded in by Step
as measurements arrive and consumed by the next physics load.
*/
type manifoldDrive struct {
	buy, sell, eta, beta float64
	hasHawkes            bool
}

/*
manifoldCut keeps the oscillators attributed to one symbol while every cut in
the market update relaxes inside the same resident domain.
*/
type manifoldCut struct {
	symbol      string
	carrier     uint32
	oscillators []Oscillator
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
	ctx context.Context,
	bus *nomagiqueruntime.Workspace,
) *Solver {
	deltaT := 0.01
	configuredDelta := viper.GetDuration("market.manifold.integration_interval")

	if configuredDelta > 0 {
		deltaT = configuredDelta.Seconds()
	}

	config := liveDomain(deltaT)

	physics, err := pmanifold.NewManifold(
		int(config.GridX),
		int(config.GridY),
		int(config.GridZ),
	)

	errnie.Error(errnie.Err(
		errnie.NotAcceptable,
		"manifold: error building manifold",
		err,
	))

	corpus, corpusErr := geometry.NewCorpus[types.PhaseOutcome](phaseCorpusCapacity)
	errnie.Error(corpusErr)
	angles, angleErr := geometry.PhasePath(phaseScanAngles)
	errnie.Error(angleErr)
	var bookSource websocket.BookSource
	var thesis *types.Thesis
	if bus != nil {
		if shared, found := bus.Shared("thesis", ""); found {
			if t, ok := shared.(*types.Thesis); ok {
				thesis = t
			}
		}
		if shared, found := bus.Shared("api", ""); found {
			if api, ok := shared.(websocket.BookSource); ok {
				bookSource = api
			}
		}
	}

	solver := &Solver{
		ctx:          ctx,
		thesis:       thesis,
		api:          bookSource,
		config:       config,
		physics:      physics,
		slabs:        slabEncoder{config: config},
		scales:       make(map[string]*adaptive.Accumulator),
		converged:    make(map[string]float64),
		priorPos:     make(map[string]map[string][3]float64),
		corpus:       corpus,
		angles:       angles,
		semaphore:    make(chan struct{}, 1),
		stopping:     make(chan struct{}),
		stopped:      make(chan struct{}),
		stepInterval: viper.GetDuration("market.manifold.step_interval"),
	}
	solver.fluid = nomagiqueruntime.ChannelOf[types.FluidFrame](
		bus, types.ChannelFluid,
		func(frame types.FluidFrame) string { return "" },
	)
	nomagiqueruntime.ChannelOf[*nmtypes.Measurement](
		bus, types.ChannelMeasurements,
		func(measurement *nmtypes.Measurement) string { return measurement.Symbol },
	).Subscribe(solver.Name(), solver.Step)

	go solver.drive()

	return solver
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

	if solver.closing.Load() || solver.ctx.Err() != nil {
		return nil
	}

	generation := solver.requested.Add(1)
	solver.latest.Store(&manifoldRequest{
		generation: generation,
		thesis:     thesis,
		at:         thesis.At,
	})
	solver.settling.Store(true)

	return nil
}

/*
Step coalesces one measurement notification into the resident request slot. The
resident domain advances on its own clock (see drive), so a burst of
notifications can never trigger more physics steps than the integration cadence
allows, and the pipeline never blocks a measurement producer.
*/
func (solver *Solver) Step(measurement *nmtypes.Measurement) error {
	if solver == nil || solver.thesis == nil || measurement == nil {
		return nil
	}

	if string(measurement.Source) == string(types.SourceHawkes) {
		entry := solver.driveFor(measurement.Symbol)
		buySample := measurement.Metrics[types.MetricKey(
			types.MetricExcitationAmplitude, types.SideBuyToBuy,
		)]
		sellSample := measurement.Metrics[types.MetricKey(
			types.MetricExcitationAmplitude, types.SideSellToSell,
		)]

		if buySample != nil && buySample.Normalized != nil {
			entry.buy = *buySample.Normalized
		}

		if sellSample != nil && sellSample.Normalized != nil {
			entry.sell = *sellSample.Normalized
		}

		if etaMetric := measurement.Metrics[types.MetricKey(
			types.MetricSpectralRadius, types.SideNone,
		)]; etaMetric != nil {
			entry.eta = etaMetric.Raw
		}

		if betaMetric := measurement.Metrics[types.MetricKey(
			types.MetricDecayRate, types.SideNone,
		)]; betaMetric != nil {
			entry.beta = betaMetric.Raw
		}

		entry.hasHawkes = true
	}

	return solver.Update(solver.thesis)
}

func (solver *Solver) driveFor(symbol string) manifoldDrive {
	loaded, _ := solver.drives.LoadOrStore(symbol, manifoldDrive{})

	return loaded.(manifoldDrive)
}

/*
drive advances the resident domain on the manifold's own clock. Every interval
it consumes the latest coalesced request generation, so step cost is bounded
and independent of how fast measurements arrive.
*/
func (solver *Solver) drive() {
	interval := solver.stepInterval

	if interval <= 0 {
		interval = manifoldStepInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	defer solver.settling.Store(false)

	for {
		select {
		case <-solver.ctx.Done():
			return
		case <-ticker.C:
			if err := solver.drainRequests(); err != nil {
				solver.err = errnie.Error(err)

				if solver.thesis != nil {
					solver.thesis.Fail(solver.err)
				}

				return
			}
		}
	}
}

/*
manifoldStepInterval is the resident domain's integration cadence. The manifold
is a physics integrator, not a per-message reducer: it settles the latest
request generation on its own clock so the step rate is bounded no matter how
fast the measurement stream runs.
*/
const manifoldStepInterval = 50 * time.Millisecond

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
			err = solver.advance(request.thesis, request.at, cuts)
		}

		if err != nil {
			return errnie.Error(err)
		}

		solver.completeRequest(request.generation)
	}
}

func (solver *Solver) nextRequest() (manifoldRequest, bool) {
	if solver.completed.Load() >= solver.requested.Load() {
		solver.settling.Store(false)
		return manifoldRequest{}, false
	}

	req := solver.latest.Load()
	if req == nil {
		return manifoldRequest{}, false
	}

	return *req, true
}

func (solver *Solver) completeRequest(generation uint64) {
	for {
		current := solver.completed.Load()
		if generation <= current {
			break
		}
		if solver.completed.CompareAndSwap(current, generation) {
			break
		}
	}
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
	drives := make(map[string]manifoldDrive)

	for _, symbolName := range solver.universe {
		if drive, found := solver.drives.Load(symbolName); found {
			drives[symbolName] = drive.(manifoldDrive)
		} else {
			drives[symbolName] = manifoldDrive{}
		}
	}

	symbolNames := make([]string, 0, len(drives))

	for symbolName := range drives {
		symbolNames = append(symbolNames, symbolName)
	}

	sort.Strings(symbolNames)
	cuts := make([]manifoldCut, 0, len(symbolNames))
	oscillators := make([]Oscillator, 0)
	etaMass := 0.0
	betaMass := 0.0
	driveMass := 0.0

	for _, symbolName := range symbolNames {
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

	solver.oscillators = oscillators
	solver.priorAt = at

	if driveMass > 0 {
		solver.driveEta = etaMass / driveMass
		solver.driveBeta = betaMass / driveMass
	}

	return cuts, nil
}

func (solver *Solver) admit(symbol string) bool {
	if _, admitted := universeIndex(solver.universe, symbol); admitted {
		return true
	}

	if uint32(len(solver.universe)) >= phaseLatticeWidth {
		return false
	}

	solver.universe = sortedUniverse(append(solver.universe, symbol))

	return true
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
) ([]Oscillator, error) {
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

	sort.Slice(orders, func(left, right int) bool {
		first := orders[left]
		second := orders[right]

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

	for index := range orders {
		ageOrder[index] = index
	}

	sort.Slice(ageOrder, func(left, right int) bool {
		if !orders[ageOrder[left]].timestamp.Equal(orders[ageOrder[right]].timestamp) {
			return orders[ageOrder[left]].timestamp.Before(orders[ageOrder[right]].timestamp)
		}

		return orders[ageOrder[left]].id < orders[ageOrder[right]].id
	})
	ageRank := make([]int, len(orders))
	queueRank := make([]int, len(orders))
	sideCount := map[mgrbook.BookDirection]int{}

	for rank, index := range ageOrder {
		ageRank[index] = rank
	}

	for index, order := range orders {
		queueRank[index] = sideCount[order.side]
		sideCount[order.side]++
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
	oscillators := make([]Oscillator, 0, len(orders))

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

		oscillators = append(oscillators, Oscillator{
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

/*
WaitingForBook answers whether Manifold has explicitly deferred any symbol
until its authoritative Level 3 manager publishes another update.
*/
func (solver *Solver) WaitingForBook() bool {
	return solver.waiting.Load()
}

func (solver *Solver) advance(
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

	fingerprint := solver.oscillators
	_, err := solver.physics.Step(oscillatorsToState(fingerprint))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("failed to advance manifold: %v", err),
			err,
		))
	}

	solver.stepped = true
	solver.reading = solver.physics.Reading()
	thesis.StoreManifold(solver.reading)
	solver.oscillators = stateToOscillators(solver.physics.State())
	solver.publishPhase(thesis, at, cuts, fingerprint)

	return solver.publishDomain()
}

func (solver *Solver) publishDomain() error {
	if solver.fluid == nil {
		return nil
	}

	fields, err := solver.slabs.Fields(solver.physics)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"manifold: failed to read resident fields",
			err,
		))
	}

	solver.fluid.Publish(types.FluidFrame{
		Channel: types.FluidFieldsChannel,
		Payload: fields,
	})
	solver.fluid.Publish(types.FluidFrame{
		Channel: types.FluidParticlesChannel,
		Payload: solver.slabs.Particles(solver.oscillators),
	})

	return nil
}

func (solver *Solver) publishPhase(
	thesis *types.Thesis,
	at time.Time,
	cuts []manifoldCut,
	fingerprint []Oscillator,
) {
	reading := solver.stampPhase(thesis, at, cuts, fingerprint)

	if solver.fluid != nil {
		solver.fluid.Publish(types.FluidFrame{
			Channel: types.FluidPhaseChannel,
			Payload: telemetry.Encode(&wire.FrameT{
				Type:  wire.FrameFluidPhaseFrame,
				Value: solver.phaseRow(at, reading, fingerprint),
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
	fingerprint []Oscillator,
) *wire.FluidPhaseFrameT {
	kuramoto := kuramotoOrderParameter(fingerprint)
	wave := latticeWave(
		fingerprint,
		solver.config.GateWidthMin(),
		solver.config.GateWidthMax(),
		int(phaseLatticeWidth),
	)
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

func kuramotoOrderParameter(oscillators []Oscillator) KuramotoSync {
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
