package manifold

import (
	"context"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
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
	ctx           context.Context
	cancel        context.CancelFunc
	advanceMu     sync.Mutex
	errMu         sync.RWMutex
	err           error
	status        *runtime.Status
	dataset       *Dataset
	physics       *sensorium.Manifold
	ObserveModule func(string, time.Duration)

	// forcing retains the latest causally-available Hawkes excitation fraction
	// per symbol. A Trade event records it; the next Level3 event lifts the
	// matching side's resting-order energy above the unit baseline. It is
	// guarded by forcingMu alone — a Trade's two-float update never waits on a
	// full physics advance, and advance reads only a coherent per-symbol
	// snapshot under the read lock.
	forcingMu sync.RWMutex
	forcing   map[string]forcingState

	// books is the venue's resident order book — the authority on what is
	// resting right now. The Level3 stream no longer carries orders at all,
	// only a semaphore saying a symbol's book moved, so the advance reads the
	// book directly rather than reconstructing one from a message tape.
	books BookSource

	wake chan struct{}

	// loaded is the set of ContentIDs the physics domain holds, and dataset is
	// the projector. Both are touched only by the advance goroutine, which is
	// the whole point of reading the book instead of a message stream: there is
	// one reader, it owns its state outright, and none of it needs a lock.
	loaded map[int64]struct{}

	// reading is the lean advance readout — resident population size and the
	// scalar Reading — published onto every Level3 envelope. One goroutine
	// publishes it and one reads it, so it is swapped as a pointer rather than
	// guarded. The resident particles and grid fields are never retained:
	// Snapshot materializes them fresh, and only for a connected viewer.
	reading atomic.Pointer[State]
	version uint64 // Owned by advanceMu, together with the published reading.

	viewer Viewer
}

/*
Viewer is the manifold's publication boundary: the observer that renders the
resident field. Wants is asked before a snapshot is materialized so a run with
no viewer attached — or a viewer whose transport is still draining the previous
frame — never pays for a full field readout it would only discard.
*/
type Viewer interface {
	WantsManifold() bool
	PublishManifold(*types.Envelope)
}

/*
BookSource is the venue's resident order book, in the two calls the manifold
needs: the symbols that have one, and a guarded read of one symbol's.

Book hands its book to the callback under the venue's own read lock, so the
callback must copy what it needs and return — it is holding a writer out for as
long as it runs. Books is only ever ranged for its keys: the values it exposes
are the live books themselves, and walking one outside Book's lock is a race.
*/
type BookSource interface {
	Books() *sync.Map
	Book(symbol string, read func(*spotbook.Book))
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

func NewSolver(ctx context.Context) *Solver {
	ctx, cancel := context.WithCancel(ctx)

	dataset := NewDataset()

	solver := &Solver{
		ctx:     ctx,
		cancel:  cancel,
		status:  runtime.NewStatus(),
		dataset: dataset,
		forcing: make(map[string]forcingState),
		loaded:  make(map[int64]struct{}),
		wake:    make(chan struct{}, 1),
		physics: sensorium.NewManifold(
			system.Cfg.Manifold.Grid.X,
			system.Cfg.Manifold.Grid.Y,
			system.Cfg.Manifold.Grid.Z,
		),
	}

	return solver
}

/*
Start launches the field advance loop. It is deliberately separate from
construction: nothing should advance — let alone publish — before the runtime
has wired the solver's viewer, and a test drives Advance directly rather than
racing a goroutine for the same pending batch.
*/
func (solver *Solver) Start() {
	go solver.run()
}

/*
SetViewer attaches the publication boundary the advance loop renders into. It
is set once during construction, before any envelope is stepped.
*/
func (solver *Solver) SetViewer(viewer Viewer) { solver.viewer = viewer }

/*
SetBooks attaches the venue's resident order book. It is set once during
construction, before any envelope is stepped.
*/
func (solver *Solver) SetBooks(books BookSource) { solver.books = books }

/*
run advances the resident field for as long as the solver lives. It is the only
goroutine that touches the physics domain on the live path: ingress coalesces
into the pending accumulator and wakes this loop, which then loads everything
observed since its last pass in a single step. Falling behind the firehose
costs resolution — more messages fold into one advance — never latency on the
market pipeline and never an unbounded backlog.
*/
func (solver *Solver) run() {
	for {
		select {
		case <-solver.ctx.Done():
			return
		case <-solver.wake:
		}

		solver.Advance()
	}
}

func (solver *Solver) Name() string { return "manifold" }

func (solver *Solver) Error() error {
	solver.errMu.RLock()
	defer solver.errMu.RUnlock()

	return solver.err
}

func (solver *Solver) halt(err error) {
	if err == nil {
		return
	}

	solver.errMu.Lock()

	if solver.err == nil {
		solver.err = err
		solver.cancel()
	}

	solver.errMu.Unlock()
}

/*
Step dispatches on the envelope kind:

  - EnvelopeTrade: if envelope.Hawkes carries valid excitation fractions, record
    them as the symbol's forcing state. No physics field advance and no
    envelope.Manifold emission happen here — the trade event only updates the
    resident forcing, never steps the domain.
  - EnvelopeLevel3: apply the message's order lifecycle and project its resting
    orders — loading the latest causally-available forcing into the resting-
    order energy — into the pending accumulator the advance loop drains. The
    field is not stepped here.
  - Any other kind is a no-op.
*/
func (solver *Solver) Step(envelope *types.Envelope) *types.Envelope {
	if solver.Error() != nil {
		solver.cancel()

		return nil
	}

	if envelope == nil {
		return envelope
	}

	switch envelope.TypeID {
	case types.EnvelopeTrade:
		symbol := ""

		if envelope.Hawkes != nil {
			symbol = envelope.Hawkes.Label
		}

		if symbol == "" {
			symbol = envelope.TradeData.Symbol
		}

		solver.recordForcing(symbol, envelope.Hawkes)

		return envelope

	case types.EnvelopeLevel3:
		// A Level3 envelope is a semaphore: it says a symbol's book moved, and
		// carries no orders. So there is nothing to project here — the ring's
		// whole obligation is to wake the advance, which then reads the book
		// itself. Every message between two advances collapses into the one
		// book state the next advance sees, which is what makes a field far
		// slower than the firehose feeding it a non-problem.
		select {
		case solver.wake <- struct{}{}:
		default:
		}

		envelope.Manifold = solver.Reading()

		return envelope
	}

	return envelope
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

	buyMetric, buyFound := hawkes.Metrics[buyExcitationMetric]
	sellMetric, sellFound := hawkes.Metrics[sellExcitationMetric]

	if !buyFound && !sellFound {
		return
	}

	buy := float32(0)
	sell := float32(0)

	if buyFound {
		if !isFiniteFloat(buyMetric.Raw) || buyMetric.Raw < 0 {
			return
		}
		buy = float32(buyMetric.Raw)
	}

	if sellFound {
		if !isFiniteFloat(sellMetric.Raw) || sellMetric.Raw < 0 {
			return
		}
		sell = float32(sellMetric.Raw)
	}

	solver.forcingMu.Lock()
	solver.forcing[symbol] = forcingState{buyExcitation: buy, sellExcitation: sell}
	solver.forcingMu.Unlock()
}

/*
latestForcing returns a symbol's retained forcing state (or the zero-value
unit baseline when none has been observed yet). It must be called while the
forcing read lock is held by advance.
*/
func (solver *Solver) latestForcing(symbol string) forcingState {
	if solver.forcing == nil {
		return forcingState{}
	}

	return solver.forcing[symbol]
}

func isFiniteFloat(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
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
func (solver *Solver) project() (departures []int64, batch *sensorium.State) {
	if solver.books == nil {
		return nil, nil
	}

	seen := make(map[int64]struct{}, len(solver.loaded))
	states := make([]*sensorium.State, 0, len(solver.loaded))
	symbols := solver.books.Books()

	if symbols == nil {
		return nil, nil
	}

	symbols.Range(func(key, _ any) bool {
		symbol, ok := key.(string)

		if !ok || symbol == "" {
			return true
		}

		solver.forcingMu.RLock()
		forcing := solver.latestForcing(symbol)
		solver.forcingMu.RUnlock()

		bids, asks := solver.readBook(symbol)

		if len(bids) == 0 && len(asks) == 0 {
			return true
		}

		for state := range solver.dataset.Step(symbol, bids, asks, forcing) {
			if state == nil || state.N != 1 {
				sensorium.StatePool.Put(state)
				continue
			}

			seen[state.ContentIDs[0]] = struct{}{}
			states = append(states, state)
		}

		return true
	})

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
readBook copies one symbol's resting orders out from under the venue's read
lock. Only the identity, price and size cross out: the projection runs on the
copy, never inside the callback.
*/
func (solver *Solver) readBook(symbol string) (bids, asks []restingOrder) {
	solver.books.Book(symbol, func(managed *spotbook.Book) {
		if managed == nil {
			return
		}

		bids = appendSide(bids, managed.Bids)
		asks = appendSide(asks, managed.Asks)
	})

	return bids, asks
}

/*
appendSide flattens one side of a book into resting orders, best price first,
preserving each level's time priority so a projected order's rank is the queue
position the venue actually gives it.
*/
func appendSide(orders []restingOrder, side *spotbook.Side) []restingOrder {
	if side == nil {
		return orders
	}

	// Best price first: a bid side is best at its high and descends, an ask
	// side is best at its low and ascends. Rank is queue position, so walking
	// a side backwards would invert every order's phase and depth.
	level := side.Low
	next := func(from *spotbook.Level) *spotbook.Level { return from.Higher }

	if side.Direction == spotbook.Bid {
		level = side.High
		next = func(from *spotbook.Level) *spotbook.Level { return from.Lower }
	}

	for ; level != nil; level = next(level) {
		if level.Price == nil {
			continue
		}

		price := level.Price.Float64()

		for _, order := range level.Queue() {
			if order == nil || order.Quantity == nil {
				continue
			}

			orders = append(orders, restingOrder{
				id:    order.ID,
				price: price,
				size:  order.Quantity.Float64(),
			})
		}
	}

	return orders
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

	if states[0].Sources != nil {
		batch.Sources = make([]sensorium.Source, count)
	}

	for index, state := range states {
		if batch.Sources != nil {
			batch.Sources[index] = state.Sources[0]
		}
		state.Sources = nil
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
func (solver *Solver) Advance() *State {
	if solver.physics == nil {
		return nil
	}

	departures, batch := solver.project()

	if len(departures) == 0 && batch == nil {
		return nil
	}

	solver.advanceMu.Lock()

	started := time.Now()
	remaining, err := solver.physics.Remove(departures)

	if err != nil {
		solver.advanceMu.Unlock()
		solver.halt(err)

		return nil
	}

	if batch == nil && remaining == 0 {
		solver.advanceMu.Unlock()

		return nil
	}

	state, err := solver.physics.Step(batch)

	if err != nil {
		solver.advanceMu.Unlock()
		solver.halt(err)

		return nil
	}

	if state == nil || state.N == 0 {
		solver.advanceMu.Unlock()

		return nil
	}

	solver.version++
	reading := State{At: time.Now(), Version: solver.version}
	reading.State.N = state.N
	reading.Reading = solver.physics.Reading()
	solver.reading.Store(&reading)
	solver.advanceMu.Unlock()

	if solver.ObserveModule != nil {
		solver.ObserveModule("manifold", time.Since(started))
	}

	solver.publish()

	return &reading
}

/*
Reading returns the lean readout of the latest advance: the resident population
size and the scalar Reading, with no particle or field arrays. It is what rides
the envelope.
*/
func (solver *Solver) Reading() *State {
	return solver.reading.Load()
}

/*
publish materializes the resident particles and fields into an envelope of their
own and hands it to the viewer. The snapshot is only taken when a viewer is
attached and its transport is ready for another frame, so a run nobody is
watching — and a viewer still draining the previous frame — costs nothing.
*/
func (solver *Solver) publish() {
	if solver.viewer == nil || !solver.viewer.WantsManifold() {
		return
	}

	snapshot := solver.Snapshot()

	if snapshot == nil {
		return
	}

	envelope := types.NewEnvelope(types.EnvelopeManifold)
	envelope.Manifold = snapshot

	solver.viewer.PublishManifold(envelope)
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
) ([]float64, *State) {
	if solver == nil || solver.physics == nil || solver.books == nil {
		return nil, nil
	}

	bids, asks := solver.readBook(symbol)
	states := make([]*sensorium.State, 0, len(bids)+len(asks))

	for state := range solver.dataset.StepClamped(symbol, bids, asks, forcing) {
		if state == nil || state.N != 1 {
			sensorium.StatePool.Put(state)
			continue
		}

		states = append(states, state)
	}

	batch := collectStates(states)

	if batch == nil || batch.N == 0 {
		return nil, nil
	}

	solver.advanceMu.Lock()
	defer solver.advanceMu.Unlock()

	for _, price := range candidateLevels {
		if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
			continue
		}

		solver.injectProbeParticle(batch, price, symbol)
	}

	if relaxationSteps <= 0 {
		relaxationSteps = 1
	}

	clampedSnapshot := snapshotClamped(batch)

	var (
		state *sensorium.State
		err   error
	)

	for step := 0; step < relaxationSteps; step++ {
		state, err = solver.physics.Step(batch)

		if err != nil {
			solver.halt(err)

			return nil, nil
		}

		solver.enforceBoundaryConditions(batch, clampedSnapshot)
	}

	reading := State{}

	if state != nil && state.N != 0 {
		solver.version++
		reading.At = time.Now()
		reading.Version = solver.version
		reading.State.N = state.N
		reading.Reading = solver.physics.Reading()
		solver.reading.Store(&reading)
	}

	return solver.extractCrystallizedProbes(batch), &reading
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
) {
	if batch == nil || symbol == "" || price <= 0 {
		return
	}

	state, _ := sensorium.StatePool.Get().(*sensorium.State)

	if state == nil {
		return
	}

	// A probe must be placed by the same resident frame the observed orders
	// were, or it crystallizes in a different coordinate system than the book
	// it is probing.
	positionX, priceDeviation := solver.dataset.frames.placePrice(
		symbol,
		math.Log(price),
	)
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

/*
Snapshot materializes the resident particles and fields for one published
frame. Nothing here is retained: PackFields gathers straight into the slices
this snapshot hands out, so the solver holds no field-sized state between
frames and a run with no viewer allocates none of it.
*/
func (solver *Solver) Snapshot() *State {
	if solver == nil || solver.physics == nil {
		return nil
	}

	solver.advanceMu.Lock()
	defer solver.advanceMu.Unlock()

	state := solver.physics.State()

	if state == nil || state.N == 0 {
		return nil
	}

	reading := solver.Reading()
	gridX, gridY, gridZ, gridSpacing := solver.physics.Grid()
	cells := gridX * gridY * gridZ

	momRho := make([]float32, cells*4)
	fieldEnergy := make([]float32, cells)
	waveReal := make([]float32, cells)
	waveImag := make([]float32, cells)

	densityScale, momentumScale, energyScale, waveScale := solver.physics.PackFields(
		momRho,
		fieldEnergy,
		waveReal,
		waveImag,
	)
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

	return &State{
		State:         cloneState(state),
		Reading:       solver.physics.Reading(),
		At:            reading.At,
		Version:       reading.Version,
		GridX:         gridX,
		GridY:         gridY,
		GridZ:         gridZ,
		GridSpacing:   gridSpacing,
		MomRho:        momRho,
		FieldEnergy:   fieldEnergy,
		WaveReal:      waveReal,
		WaveImag:      waveImag,
		DensityScale:  densityScale,
		MomentumScale: momentumScale,
		EnergyScale:   energyScale,
		WaveScale:     waveScale,
		Modes:         modes,
	}
}

func cloneState(state *sensorium.State) sensorium.State {
	n := state.N
	return sensorium.State{
		N:          n,
		Bytes:      append([]int64(nil), state.Bytes[:n]...),
		Seqs:       append([]int64(nil), state.Seqs[:n]...),
		TokenIDs:   append([]int64(nil), state.TokenIDs[:n]...),
		ContentIDs: append([]int64(nil), state.ContentIDs[:n]...),
		Phase:      append([]float32(nil), state.Phase[:n]...),
		Omega:      append([]float32(nil), state.Omega[:n]...),
		Energy:     append([]float32(nil), state.Energy[:n]...),
		Mass:       append([]float32(nil), state.Mass[:n]...),
		Heat:       append([]float32(nil), state.Heat[:n]...),
		Amp:        append([]float32(nil), state.Amp[:n]...),
		Pos:        append([]float32(nil), state.Pos[:n*3]...),
		Vel:        append([]float32(nil), state.Vel[:n*3]...),
		Clamped:    append([]bool(nil), state.Clamped[:n]...),
		Dark:       append([]bool(nil), state.Dark[:n]...),
	}
}

func (solver *Solver) Close() error {
	if solver == nil {
		return nil
	}

	solver.advanceMu.Lock()
	defer solver.advanceMu.Unlock()

	if solver.cancel != nil {
		solver.cancel()
	}

	if solver.physics != nil {
		solver.physics.Close()
		solver.physics = nil
	}

	return nil
}
