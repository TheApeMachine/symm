package manifold

import (
	"context"
	"fmt"
	"math"
	goruntime "runtime"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/nomagique/relation"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

// WaveMode and State are defined in types so types.Envelope can carry a
// manifold advance without an import cycle back to logic/manifold.
type WaveMode = types.WaveMode
type State = types.ManifoldState

/*
Solver owns one resident Sensorium domain for the complete market universe.
Symbols contribute orders to the same gas and wave fields; they are not split
into independent simulations that cannot interfere.

It satisfies nomagique/runtime.Node[*types.Envelope]. A Level3 envelope projects
its message's orders into oscillators exactly once and hands them to the field,
forward only. There is no retained book and no separate trade trigger: the
Level3 stream is the whole input.

Projection and the field advance run on different goroutines, because one GPU
field step is orders of magnitude slower than the Level3 stream feeding it. Step
does the message-local work and returns; the advance loop (see Start) loads
everything observed since its previous pass and steps once. Falling behind the
firehose therefore costs resolution — more messages fold into one advance —
never latency on the market pipeline, and the accumulator is bounded by the
number of live orders rather than by the message rate.
*/
type Solver struct {
	*runtime.System
	arena       *data.ArenaOwner
	isAdvancing atomic.Bool
	book        *broker.Book
	dataset     *Dataset
	physics     *sensorium.Manifold
	forcing     sync.Map
	wake        chan struct{}
	dirty       sync.Map
	loaded      map[int64]string
	reading     atomic.Pointer[State]
	version     atomic.Uint64
}

/*
forcingState is one symbol's retained Hawkes excitation fractions above the
unit oscillator baseline: buy excitation lifts ask-side resting orders, sell
excitation lifts bid-side resting orders.
*/
type forcingState struct {
	buyExcitation  float32
	sellExcitation float32
}

/*
forcingInputs is the declared coordinate contract for Manifold forcing. The
lookup keys below are built from these selectors once during package setup, so
runtime reads and generated metric lineage cannot drift into separate names.
*/
var forcingInputs = struct {
	Buy  relation.Selector
	Sell relation.Selector
}{
	Buy: relation.Selector{
		Source: "hawkes",
		Metric: "excitation_fraction",
		Side:   "buy",
	},
	Sell: relation.Selector{
		Source: "hawkes",
		Metric: "excitation_fraction",
		Side:   "sell",
	},
}

var (
	buyExcitationMetric  = forcingInputs.Buy.Metric + ":" + forcingInputs.Buy.Side
	sellExcitationMetric = forcingInputs.Sell.Metric + ":" + forcingInputs.Sell.Side
)

func NewSolver(ctx context.Context, arena *data.ArenaOwner, book *broker.Book) *Solver {
	solver := &Solver{
		arena:   arena,
		book:    book,
		dataset: NewDataset(),
		loaded:  make(map[int64]string),
		wake:    make(chan struct{}, 1),
		physics: sensorium.NewManifold(
			system.Cfg.Manifold.Grid.X,
			system.Cfg.Manifold.Grid.Y,
			system.Cfg.Manifold.Grid.Z,
		),
	}

	solver.System = runtime.NewSystem(ctx, "manifold", solver.physics)
	solver.Transition(runtime.READY)
	return solver
}

func (solver *Solver) Arena() *data.ArenaOwner {
	return solver.arena
}

/*
Start launches the field advance loop. It is deliberately separate from
construction: nothing should advance — let alone publish — before the runtime
has activated the output tees, and a test drives Advance directly rather than
racing a goroutine for the same pending batch.
*/
func (solver *Solver) Start() {
	go solver.run()
}

/*
RecordForcing records Hawkes excitation fractions for the given symbol directly.
*/
func (solver *Solver) RecordForcing(symbol string, hawkes *data.Measurement[float64]) {
	solver.recordForcing(symbol, hawkes)
}

/*
Wake marks the symbol dirty and wakes the field advance loop.
*/
func (solver *Solver) Wake(symbol string) {
	solver.markDirty(symbol)

	select {
	case solver.wake <- struct{}{}:
	default:
	}
}

/*
run advances the resident field for as long as the solver lives. It is the only
goroutine that touches the physics domain on the live path: ingress coalesces
into the pending accumulator and wakes this loop, which then loads everything
observed since its last pass in a single step. Falling behind the firehose
costs resolution — more messages fold into one advance — never latency on the
market pipeline and never an unbounded backlog.
*/
func (solver *Solver) run() {
	ticker := time.NewTicker(33 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-solver.Context().Done():
			return
		case <-solver.wake:
		case <-ticker.C:
		}

		// Advance is synchronous. Once it returns the GPU work is complete;
		// a wake received during that work remains queued for the next pass.
		solver.Advance()
	}
}

/*
Step dispatches on the envelope kind:
*/
func (solver *Solver) Step(prior *data.Measurement[float64]) *data.Measurement[float64] {
	if solver.Status() != runtime.READY {
		return nil
	}

	if prior == nil {
		return nil
	}

	symbol := prior.Label

	var priors []*data.Measurement[float64]
	if prior.Source == "runtime:join" {
		priors = prior.Peers
	} else {
		priors = append([]*data.Measurement[float64]{prior}, prior.Peers...)
	}

	// Process hawkes forcing from prior-stage outputs.
	for _, p := range priors {
		if p == nil {
			continue
		}

		if p.Source == "hawkes:trade" || p.Source == "hawkes" {
			solver.recordForcing(p.Label, p)
		}

		if p.Label == "" {
			continue
		}

		solver.markDirty(p.Label)

		if symbol == "" {
			symbol = p.Label
		}
	}

	select {
	case solver.wake <- struct{}{}:
	default:
	}

	out := solver.arena.NewMeasurement(solver.Name())
	out.Epoch = prior.Epoch
	out.Tick = prior.Tick
	out.Label = symbol
	out.SeqIdx = prior.SeqIdx
	out.At = prior.At
	out.From = prior.From
	out.Peers = []*data.Measurement[float64]{prior}

	reading := solver.Reading()
	out.Result = reading

	if reading == nil {
		out.Finalize()
		return out
	}

	out.SetMetric("divergence", data.NewMetric[float64](
		"divergence",
		data.UnitRate,
		data.TimescaleInstantaneous,
		0.0,
		1.0,
	).Write(reading.Reading.Divergence))
	out.SetMetric("guidance_speed", data.NewMetric[float64](
		"guidance_speed",
		data.UnitVelocity,
		data.TimescaleInstantaneous,
		0.0,
		1.0,
	).Write(reading.Reading.GuidanceSpeed))
	out.SetMetric("coherence_mag2", data.NewMetric[float64](
		"coherence_mag2",
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
		0.5,
		0.5,
	).Write(reading.Reading.CoherenceMag2))
	out.SetMetric("pressure_grad_norm", data.NewMetric[float64](
		"pressure_grad_norm",
		data.UnitAcceleration,
		data.TimescaleInstantaneous,
		0.0,
		1.0,
	).Write(reading.Reading.PressureGradNorm))
	out.SetMetric("viscosity_proxy", data.NewMetric[float64](
		"viscosity_proxy",
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
		0.0,
		1.0,
	).Write(reading.Reading.ViscosityProxy))
	out.SetMetric("kuramoto_r", data.NewMetric[float64](
		"kuramoto_r",
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
		0.5,
		0.5,
	).Write(reading.Reading.KuramotoR))
	out.SetMetric("kuramoto_psi", data.NewMetric[float64](
		"kuramoto_psi",
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
		0.0,
		math.Pi,
	).Write(reading.Reading.KuramotoPsi))
	out.SetMetric("gas_kinetic", data.NewMetric[float64](
		"gas_kinetic",
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
		0.0,
		1.0,
	).Write(reading.Reading.Health.Gas.Kinetic))
	out.SetMetric("gas_internal", data.NewMetric[float64](
		"gas_internal",
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
		0.0,
		1.0,
	).Write(reading.Reading.Health.Gas.Internal))
	out.SetMetric("wave_norm", data.NewMetric[float64](
		"wave_norm",
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
		1.0,
		1.0,
	).Write(reading.Reading.Health.Wave.Norm))
	out.SetMetric("vorticity_rms", data.NewMetric[float64](
		"vorticity_rms",
		data.UnitRate,
		data.TimescaleInstantaneous,
		0.0,
		1.0,
	).Write(reading.Reading.Health.Gas.VorticityRMS))
	out.SetMetric("strain_rms", data.NewMetric[float64](
		"strain_rms",
		data.UnitRate,
		data.TimescaleInstantaneous,
		0.0,
		1.0,
	).Write(reading.Reading.Health.Gas.StrainRMS))
	out.SetMetric("max_mach", data.NewMetric[float64](
		"max_mach",
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
		0.0,
		1.0,
	).Write(reading.Reading.Health.Gas.MaxMach))

	if reading.State != nil {
		n := float64(reading.State.N)
		out.SetMetric("particle_count", data.NewMetric[float64](
			"particle_count",
			data.UnitCount,
			data.TimescaleInstantaneous,
			n,
			math.Sqrt(n),
		).Write(n))
	}

	out.SetMetric("particle_thermal", data.NewMetric[float64](
		"particle_thermal",
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
		0.0,
		1.0,
	).Write(reading.Reading.Health.ParticleThermal))
	out.SetMetric("particle_kinetic", data.NewMetric[float64](
		"particle_kinetic",
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
		0.0,
		1.0,
	).Write(reading.Reading.Health.ParticleKinetic))

	maturity, snr, snrDefined, estimated := out.Maturity, out.SNR, out.SNRDefined, out.Estimated
	if reading.State != nil && reading.State.N > 1 {
		maturity = 1.0 - 1.0/float64(reading.State.N)
	}

	if reading.Reading.KuramotoR > 0 {
		r2 := reading.Reading.KuramotoR * reading.Reading.KuramotoR
		denom := 1.0 - r2

		if denom > 0 {
			snr = r2 / denom
			snrDefined = true
			estimated = true
		}
	}
	out.SetQuality(maturity, snr, snrDefined, estimated)
	out.Finalize()

	return out
}

/*
recordForcing stores the symbol's latest Hawkes excitation fractions under the
forcing lock alone. A non-finite or invalid fraction is rejected rather than
silently poisoning resident forcing state. Trade events never advance the field,
so this path never contends with the physics advance lock.
*/
func (solver *Solver) recordForcing(symbol string, hawkes *data.Measurement[float64]) {
	if hawkes == nil || hawkes.Err != nil || symbol == "" {
		return
	}

	buyMetric, buyFound := hawkes.LookupMetric(buyExcitationMetric)
	sellMetric, sellFound := hawkes.LookupMetric(sellExcitationMetric)

	if !buyFound && !sellFound {
		return
	}

	buy := float32(0)
	sell := float32(0)

	if buyFound {
		if buyMetric.Raw < 0 {
			return
		}
		buy = float32(buyMetric.Raw)
	}

	if sellFound {
		if sellMetric.Raw < 0 {
			return
		}
		sell = float32(sellMetric.Raw)
	}

	solver.forcing.Store(symbol, forcingState{buyExcitation: buy, sellExcitation: sell})
}

/*
project reads every resting order the venue holds and turns it into one batch.

The book is the population, so this is also what defines residency: an order
absent from the book has left it, whatever the reason and whether or not any
message announced it. Departures are the difference between what physics holds
and what the book just showed, which needs no order lifecycle to reconstruct —
the venue already maintains one, and reconstructing a second from a message tape
only creates something that can disagree with it.

The venue's lock is held for the copy of each symbol's orders and released
before they are projected, so a book writer never waits on the projection math.
*/
func (solver *Solver) markDirty(symbol string) {
	if symbol == "" {
		return
	}

	solver.dirty.Store(symbol, struct{}{})
}

func (solver *Solver) project() (departures []int64, batch *sensorium.State) {
	if solver.book == nil {
		return nil, nil
	}

	seen := make(map[int64]string, len(solver.loaded))
	for contentID, symbol := range solver.loaded {
		seen[contentID] = symbol
	}
	states := make([]*sensorium.State, 0, len(solver.loaded))

	dirtySymbols := make([]string, 0)
	solver.dirty.Range(func(key, _ any) bool {
		if symbol, ok := key.(string); ok && symbol != "" {
			dirtySymbols = append(dirtySymbols, symbol)
		}
		return true
	})

	for _, sym := range dirtySymbols {
		solver.dirty.Delete(sym)
	}

	if len(dirtySymbols) == 0 && len(solver.loaded) == 0 {
		if all := solver.book.All(); all != nil {
			all.Range(func(key, _ any) bool {
				if symbol, ok := key.(string); ok && symbol != "" {
					dirtySymbols = append(dirtySymbols, symbol)
				}
				return true
			})
		}
	}

	for _, symbol := range dirtySymbols {
		for contentID, residentSymbol := range seen {
			if residentSymbol == symbol {
				delete(seen, contentID)
			}
		}
		var forcingVal forcingState
		if f, ok := solver.forcing.Load(symbol); ok {
			forcingVal, _ = f.(forcingState)
		}

		solver.book.Book(symbol, func(b *spotbook.Book) {
			if b == nil || b.Bids == nil || b.Asks == nil {
				return
			}

			for state := range solver.dataset.Step(symbol, b.Bids, b.Asks, forcingVal) {
				if state == nil {
					continue
				}

				if len(state.ContentIDs) > 0 {
					seen[state.ContentIDs[0]] = symbol
				}
				states = append(states, state)
			}
		})
	}

	if solver.dataset.Error() != nil {
		for _, state := range states {
			sensorium.StatePool.Put(state)
		}
		return nil, nil
	}

	for contentID := range solver.loaded {
		if _, resting := seen[contentID]; !resting {
			departures = append(departures, contentID)
		}
	}

	// The physics domain removes by identity, but a sorted list keeps one
	// advance's eviction order reproducible across runs.
	sort.Slice(departures, func(left, right int) bool {
		return departures[left] < departures[right]
	})

	batch = collectStates(states)
	solver.loaded = seen

	return departures, batch
}

/*
collectStates packs the per-order States the projector yielded into one batch
and returns each to the pool. One batch per advance is the whole point of
reading the book: the domain is loaded once, however many messages moved it.
*/
func collectStates(states []*sensorium.State) *sensorium.State {
	if len(states) == 0 {
		return nil
	}

	count := len(states)
	batch := &sensorium.State{
		N:          count,
		Bytes:      make([]int64, count),
		Seqs:       make([]int64, count),
		TokenIDs:   make([]int64, count),
		ContentIDs: make([]int64, count),
		Phase:      make([]float32, count),
		Omega:      make([]float32, count),
		Energy:     make([]float32, count),
		Mass:       make([]float32, count),
		Heat:       make([]float32, count),
		Amp:        make([]float32, count),
		Pos:        make([]float32, count*3),
		Vel:        make([]float32, count*3),
		Clamped:    make([]bool, count),
		Dark:       make([]bool, count),
	}

	for index, state := range states {
		batch.Bytes[index] = state.Bytes[0]
		batch.Seqs[index] = state.Seqs[0]
		batch.TokenIDs[index] = state.TokenIDs[0]
		batch.ContentIDs[index] = state.ContentIDs[0]
		batch.Phase[index] = state.Phase[0]
		batch.Omega[index] = state.Omega[0]
		batch.Energy[index] = state.Energy[0]
		batch.Mass[index] = state.Mass[0]
		batch.Heat[index] = state.Heat[0]
		batch.Amp[index] = state.Amp[0]
		batch.Pos[index*3+0] = state.Pos[0]
		batch.Pos[index*3+1] = state.Pos[1]
		batch.Pos[index*3+2] = state.Pos[2]
		batch.Vel[index*3+0] = state.Vel[0]
		batch.Vel[index*3+1] = state.Vel[1]
		batch.Vel[index*3+2] = state.Vel[2]
		batch.Clamped[index] = state.Clamped[0]
		batch.Dark[index] = state.Dark[0]
		sensorium.StatePool.Put(state)
	}

	return batch
}

/*
Advance loads everything observed since the previous pass into the resident
domain and steps the field exactly once, however many Level3 messages that
covers. It is the run loop's body, exported so a test can drive the advance
deterministically instead of waiting on the goroutine.
*/
func (solver *Solver) Advance() {
	if !solver.isAdvancing.CompareAndSwap(false, true) {
		return
	}
	defer solver.isAdvancing.Store(false)

	departures, batch := solver.project()

	if err := solver.dataset.Error(); err != nil {
		solver.Error(err)
		return
	}

	_, err := solver.physics.Remove(departures)

	if err != nil {
		solver.Error(err)

		return
	}

	if len(solver.loaded) == 0 && batch == nil {
		return
	}

	state, err := solver.physics.Step(batch)

	if err != nil {
		solver.Error(err)
		return
	}

	if state != nil {
		solver.publishReading(state)
	}
}

/*
computePhaseResultants computes each book side's amplitude-weighted Kuramoto vector
directly from the resident oscillator particles.
Side is determined by the token ID (even is bid, odd is ask).
*/
func computePhaseResultants(state *sensorium.State) []types.PhaseChannelResultant {
	if state == nil || state.N == 0 {
		return nil
	}

	var bidReal, bidImag, bidTotalAmp float64
	var bidCount int
	var askReal, askImag, askTotalAmp float64
	var askCount int

	for index := 0; index < state.N; index++ {
		phase := float64(state.Phase[index])
		amp := float64(state.Amp[index])
		cosP := math.Cos(phase)
		sinP := math.Sin(phase)

		if (state.TokenIDs[index] & 1) == 0 {
			bidReal += amp * cosP
			bidImag += amp * sinP
			bidTotalAmp += amp
			bidCount++
			continue
		}

		askReal += amp * cosP
		askImag += amp * sinP
		askTotalAmp += amp
		askCount++
	}

	bidCoherence := 0.0
	bidPhase := 0.0

	if bidTotalAmp > 0 {
		bidCoherence = math.Hypot(bidReal, bidImag) / bidTotalAmp
	}

	if bidCount > 0 {
		bidPhase = math.Atan2(bidImag, bidReal)
	}

	askCoherence := 0.0
	askPhase := 0.0

	if askTotalAmp > 0 {
		askCoherence = math.Hypot(askReal, askImag) / askTotalAmp
	}

	if askCount > 0 {
		askPhase = math.Atan2(askImag, askReal)
	}

	return []types.PhaseChannelResultant{
		{
			Side:           "bid",
			Count:          bidCount,
			TotalAmplitude: bidTotalAmp,
			Coherence:      bidCoherence,
			Phase:          bidPhase,
		},
		{
			Side:           "ask",
			Count:          askCount,
			TotalAmplitude: askTotalAmp,
			Coherence:      askCoherence,
			Phase:          askPhase,
		},
	}
}

func (solver *Solver) publishReading(state *sensorium.State) *State {
	modeOmega, modeReal, modeImag, modeLinewidth := solver.physics.SpectralModes()
	modes := make([]WaveMode, len(modeOmega))

	for index := range modeOmega {
		modes[index] = WaveMode{
			Omega:     modeOmega[index],
			Real:      modeReal[index],
			Imag:      modeImag[index],
			Linewidth: modeLinewidth[index],
		}
	}

	gridX, gridY, gridZ, gridSpacing := solver.physics.Grid()
	cells := gridX * gridY * gridZ
	momRho := make([]float32, cells*4)
	fieldEnergy := make([]float32, cells)
	waveReal := make([]float32, cells)
	waveImag := make([]float32, cells)
	densityScale, momentumScale, energyScale, waveScale := solver.physics.PackFields(
		momRho, fieldEnergy, waveReal, waveImag,
	)

	version := solver.version.Add(1)
	reading := State{
		At:      time.Now(),
		Version: version,
		State: &sensorium.State{
			N:                 state.N,
			CoherencePosition: slices.Clone(state.CoherencePosition),
			Bytes:             slices.Clone(state.Bytes),
			Seqs:              slices.Clone(state.Seqs),
			TokenIDs:          slices.Clone(state.TokenIDs),
			ContentIDs:        slices.Clone(state.ContentIDs),
			Phase:             slices.Clone(state.Phase),
			Omega:             slices.Clone(state.Omega),
			Energy:            slices.Clone(state.Energy),
			Mass:              slices.Clone(state.Mass),
			Heat:              slices.Clone(state.Heat),
			MaterialEnergy:    slices.Clone(state.MaterialEnergy),
			Amp:               slices.Clone(state.Amp),
			Pos:               slices.Clone(state.Pos),
			Vel:               slices.Clone(state.Vel),
			PilotVel:          slices.Clone(state.PilotVel),
			PhasePotential:    slices.Clone(state.PhasePotential),
			Clamped:           slices.Clone(state.Clamped),
			Dark:              slices.Clone(state.Dark),
		},
		GridX: gridX, GridY: gridY, GridZ: gridZ, GridSpacing: gridSpacing,
		MomRho: momRho, FieldEnergy: fieldEnergy, WaveReal: waveReal, WaveImag: waveImag,
		DensityScale: densityScale, MomentumScale: momentumScale,
		EnergyScale: energyScale, WaveScale: waveScale,
		Reading:    solver.physics.Reading(),
		Modes:      modes,
		Resultants: computePhaseResultants(state),
	}
	solver.reading.Store(&reading)

	return &reading
}

/*
Reading returns the immutable particle, spectral and scalar readout of the
latest advance, including the complete Eulerian grids.
*/
func (solver *Solver) Reading() *State {
	return solver.reading.Load()
}

/*
Crystallize runs one active BVP relaxation: the message's resting orders are
clamped boundary particles, candidateLevels are injected as unclamped dark
probe particles, and the field is stepped relaxationSteps times with clamped
particles restored after each step. The returned []float64 is the settled
Omega of the surviving probe particles — the crystallized frequency readout —
together with the final manifold State.
*/
func (solver *Solver) Crystallize(
	symbol string,
	forcing forcingState,
	candidateLevels []float64,
	relaxationSteps int,
) []float64 {
	states := make([]*sensorium.State, 0)

	if solver.book != nil {
		solver.book.Book(symbol, func(b *spotbook.Book) {
			if b == nil || b.Bids == nil || b.Asks == nil {
				return
			}

			for state := range solver.dataset.StepClamped(symbol, b.Bids, b.Asks, forcing) {
				if state != nil {
					states = append(states, state)
				}
			}
		})
	}

	if err := solver.dataset.Error(); err != nil {
		for _, state := range states {
			sensorium.StatePool.Put(state)
		}
		return nil
	}
	batch := collectStates(states)

	if batch == nil || batch.N == 0 {
		return nil
	}

	for !solver.isAdvancing.CompareAndSwap(false, true) {
		goruntime.Gosched()
	}
	defer solver.isAdvancing.Store(false)

	for _, price := range candidateLevels {
		if price <= 0 {
			continue
		}

		if err := solver.injectProbeParticle(batch, price, symbol); err != nil {
			return nil
		}
	}

	if relaxationSteps <= 0 {
		relaxationSteps = 1
	}

	clampedSnapshot := snapshotClamped(batch)

	var err error
	for step := 0; step < relaxationSteps; step++ {
		_, err = solver.physics.Step(batch)

		if err != nil {
			return nil
		}

		solver.enforceBoundaryConditions(batch, clampedSnapshot)
	}

	return solver.extractCrystallizedProbes(batch)
}

/*
injectProbeParticle appends one unclamped dark particle at a candidate price.
The particle starts with unit oscillator energy and positive heat so it can
explore the field before settling; its position is the candidate's log-price
deviation in the same frame the message's resting orders use.
*/
func (solver *Solver) injectProbeParticle(
	batch *sensorium.State,
	price float64,
	symbol string,
) error {
	if batch == nil || symbol == "" || !(price > 0) {
		return fmt.Errorf("manifold: batch, symbol and positive probe price required")
	}

	// A probe must be placed by the same resident frame the observed orders
	// were, or it crystallizes in a different coordinate system than the book
	// it is probing.
	positionX, priceDeviation, err := solver.dataset.frames.placePrice(
		symbol,
		math.Log(price),
	)
	if err != nil {
		return err
	}
	state := sensorium.StatePool.Get().(*sensorium.State)
	symbolIndex := symbolToken(symbol)
	token := packToken(symbolIndex, 0)

	state.N = 1
	state.Bytes[0] = int64(token)
	state.Seqs[0] = int64(batch.N)
	state.TokenIDs[0] = int64(token)
	state.ContentIDs[0] = int64(symbolIndex)
	state.Phase[0] = 0
	state.Omega[0] = float32(math.Tanh(priceDeviation) * omegaHalfSpan)
	// A probe is a price the book has not stated an order for, so it has no
	// size of its own and carries the unit energy that makes it a light,
	// neutral test particle. Mass tracks energy as it does for every particle.
	state.Energy[0] = unitProbeEnergy
	state.Mass[0] = unitProbeEnergy
	state.Heat[0] = 0.5
	state.Amp[0] = float32(math.Sqrt(float64(state.Energy[0])))
	state.Pos[0] = float32(positionX)
	state.Pos[1] = 0.5
	state.Pos[2] = 0.5
	state.Vel[0] = 0
	state.Vel[1] = 0
	state.Vel[2] = 0
	state.Clamped[0] = false
	state.Dark[0] = true

	batch.Bytes = append(batch.Bytes, state.Bytes[0])
	batch.Seqs = append(batch.Seqs, state.Seqs[0])
	batch.TokenIDs = append(batch.TokenIDs, state.TokenIDs[0])
	batch.ContentIDs = append(batch.ContentIDs, state.ContentIDs[0])
	batch.Phase = append(batch.Phase, state.Phase[0])
	batch.Omega = append(batch.Omega, state.Omega[0])
	batch.Energy = append(batch.Energy, state.Energy[0])
	batch.Mass = append(batch.Mass, state.Mass[0])
	batch.Heat = append(batch.Heat, state.Heat[0])
	batch.Amp = append(batch.Amp, state.Amp[0])
	batch.Pos = append(batch.Pos, state.Pos[0], state.Pos[1], state.Pos[2])
	batch.Vel = append(batch.Vel, state.Vel[0], state.Vel[1], state.Vel[2])
	batch.Clamped = append(batch.Clamped, state.Clamped[0])
	batch.Dark = append(batch.Dark, state.Dark[0])
	batch.N++

	sensorium.StatePool.Put(state)
	return nil
}

/*
snapshotClamped captures the boundary values of every clamped particle so a
relaxation step can restore them after the physics step has moved them.
*/
func snapshotClamped(batch *sensorium.State) []float32 {
	if batch == nil || batch.N == 0 {
		return nil
	}

	snapshot := make([]float32, 0, batch.N*10)

	for index := 0; index < batch.N; index++ {
		if !batch.Clamped[index] {
			continue
		}

		snapshot = append(snapshot,
			batch.Pos[index*3+0],
			batch.Pos[index*3+1],
			batch.Pos[index*3+2],
			batch.Vel[index*3+0],
			batch.Vel[index*3+1],
			batch.Vel[index*3+2],
			batch.Phase[index],
			batch.Omega[index],
			batch.Energy[index],
			batch.Amp[index],
		)
	}

	return snapshot
}

/*
enforceBoundaryConditions restores clamped particles to their observed boundary
values after one relaxation step.
*/
func (solver *Solver) enforceBoundaryConditions(batch *sensorium.State, snapshot []float32) {
	if batch == nil || batch.N == 0 || len(snapshot) == 0 {
		return
	}

	cursor := 0

	for index := 0; index < batch.N; index++ {
		if !batch.Clamped[index] {
			continue
		}

		if cursor+10 > len(snapshot) {
			return
		}

		batch.Pos[index*3+0] = snapshot[cursor]
		batch.Pos[index*3+1] = snapshot[cursor+1]
		batch.Pos[index*3+2] = snapshot[cursor+2]
		batch.Vel[index*3+0] = snapshot[cursor+3]
		batch.Vel[index*3+1] = snapshot[cursor+4]
		batch.Vel[index*3+2] = snapshot[cursor+5]
		batch.Phase[index] = snapshot[cursor+6]
		batch.Omega[index] = snapshot[cursor+7]
		batch.Energy[index] = snapshot[cursor+8]
		batch.Amp[index] = snapshot[cursor+9]
		cursor += 10
	}
}

/*
extractCrystallizedProbes reads out the settled Omega of every surviving dark,
unclamped probe particle.
*/
func (solver *Solver) extractCrystallizedProbes(batch *sensorium.State) []float64 {
	if batch == nil || batch.N == 0 {
		return nil
	}

	predictions := make([]float64, 0, batch.N)

	for index := 0; index < batch.N; index++ {
		if batch.Clamped[index] || !batch.Dark[index] {
			continue
		}

		predictions = append(predictions, float64(batch.Omega[index]))
	}

	return predictions
}
