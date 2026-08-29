package manifold

import (
	"iter"
	"math"
	"sort"
	"sync"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/system"
)

/*
Dataset projects the resident Level3 order book into Sensorium States. Every
resting order becomes one oscillator carrying the structure the shared book
actually holds, not a byte-valued summary of it.

The book itself is Dataset's own accumulated state, maintained one message at
a time by Step as Level3Data events stream through the Level3 workload — never
a fully reconstructed venue book read from anywhere external. Generate walks
that accumulated state; nothing here ever reads a shared/injected book.

The mapping follows what the physics consumes directly:

  - Position X: relative log price log(price/mid), bids below asks.
  - Position Y: log order quantity, so liquidity spans orders of magnitude.
  - Position Z: queue age rank, oldest order at the front.
  - Mass:      one carrier unit per resting order; quantity lives in Y, not mass.
  - Heat:      a deterministic initial thermal store, the coupling fuel the
    coherence broadcast spends; collisions and Planck relaxation sustain it.
  - Energy:    one unit per order; the Hawkes side self-excitation is the forcing
    term that lifts an excited side above unit energy.
  - Phase:     the bid/ask spread as a quantum boundary — bids [0, π), asks
    [π, 2π) — swept by price-time queue priority within each side.
  - Omega:     log distance from mid, normalized by the symbol's own accumulated
    scale and mapped through tanh, so a resting order keeps a stable
    frequency across ticks instead of being re-derived from the batch
    it happens to share.
*/
type Dataset struct {
	mutex     sync.Mutex
	books     map[string]*symbolBook
	scales    map[string]*scaleAccumulator
	converged map[string]float64
}

/*
bookSide is this side's currently resting orders, keyed by order ID.
*/
type bookSide map[string]kraken.Level3Order

/*
symbolBook is one symbol's accumulated resting-order state, maintained purely
by applying Level3Data add/modify/delete events as they arrive. It is never a
snapshot fetched from anywhere else.
*/
type symbolBook struct {
	bids bookSide
	asks bookSide
}

/*
apply folds one Level3Data message's bid/ask events into this symbol's
resident order state: add/modify set the order's current resting entry
(order_qty is the new absolute remaining quantity, not a delta), delete
removes it.
*/
func (book *symbolBook) apply(message kraken.Level3Data) {
	applySide(book.bids, message.Bids)
	applySide(book.asks, message.Asks)
}

func applySide(side bookSide, orders []kraken.Level3Order) {
	for _, order := range orders {
		if order.Event == "delete" {
			delete(side, order.OrderID)
			continue
		}

		if order.LimitPrice == nil || order.OrderQty == nil {
			continue
		}

		side[order.OrderID] = order
	}
}

/*
scaleAccumulator is a running sum-of-squares and count used to converge a
symbol's Ω normalization scale. It is a plain Kahan-style accumulation; the
frame-based adaptive primitives are not needed for a single scalar.
*/
type scaleAccumulator struct {
	total float64
	carry float64
	count int
}

func (accumulator *scaleAccumulator) add(sample float64) {
	compensated := sample - accumulator.carry
	next := accumulator.total + compensated
	accumulator.carry = next - accumulator.total - compensated
	accumulator.total = next
	accumulator.count++
}

func (accumulator *scaleAccumulator) rms() float64 {
	if accumulator.count == 0 {
		return 0
	}

	return math.Sqrt(accumulator.total / float64(accumulator.count))
}

/*
orderEntry couples one resident order with the side it rests on.
*/
type orderEntry struct {
	order kraken.Level3Order
	ask   bool
}

const (
	unitCarrierMass      = float32(1)
	unitOscillatorEnergy = float32(1)
	symbolIndexMask      = uint32(0x7fff)
	omegaHalfSpan        = 4.0
)

func NewDataset() *Dataset {
	return &Dataset{
		books:     make(map[string]*symbolBook),
		scales:    make(map[string]*scaleAccumulator),
		converged: make(map[string]float64),
	}
}

func (dataset *Dataset) Name() string { return "book" }

/*
Step folds one Level3Data message into this symbol's accumulated resident
order state. It is the sole way Dataset's book state ever changes.
*/
func (dataset *Dataset) Step(message kraken.Level3Data) {
	if dataset == nil || message.Symbol == "" {
		return
	}

	dataset.mutex.Lock()
	defer dataset.mutex.Unlock()

	book, found := dataset.books[message.Symbol]

	if !found {
		book = &symbolBook{bids: bookSide{}, asks: bookSide{}}
		dataset.books[message.Symbol] = book
	}

	book.apply(message)
}

/*
Generate yields one State per resident order across every symbol's
accumulated book, ask side first then bid side, level by level, in queue
order. The book is Dataset's own state, built exclusively by Step.
*/
func (dataset *Dataset) Generate() iter.Seq[*sensorium.State] {
	return func(yield func(*sensorium.State) bool) {
		if dataset == nil {
			return
		}

		dataset.mutex.Lock()
		symbols := make([]string, 0, len(dataset.books))
		booksBySymbol := make(map[string]*symbolBook, len(dataset.books))

		for symbol, book := range dataset.books {
			symbols = append(symbols, symbol)
			booksBySymbol[symbol] = book
		}
		dataset.mutex.Unlock()

		sort.Strings(symbols)

		for _, symbol := range symbols {
			book := booksBySymbol[symbol]
			entries := sideEntries(book)
			mid := midPrice(book)

			if mid <= 0 || len(entries) == 0 {
				continue
			}

			symbolIndex, found := universeIndex(symbols, symbol)

			if !found {
				continue
			}

			scale := dataset.omegaScale(symbol, entries, mid)
			ranks := ageRanks(entries)
			queue := queueRanks(entries)

			grid := manifoldGrid()

			for seq, entry := range entries {
				price := entry.order.LimitPrice.Float64()
				quantity := entry.order.OrderQty.Float64()

				if price <= 0 || quantity <= 0 {
					continue
				}

				state := orderState(
					entry, seq,
					price, quantity, mid, scale,
					ranks[seq], queue[seq], len(entries),
					uint32(symbolIndex), grid,
				)

				if !yield(state) {
					return
				}
			}
		}
	}
}

/*
sideEntries flattens one symbol's accumulated book into ask-then-bid entries,
each tagged with its side, sorted by order ID so iteration never depends on Go
map ordering.
*/
func sideEntries(book *symbolBook) []orderEntry {
	entries := make([]orderEntry, 0, len(book.asks)+len(book.bids))

	entries = appendSide(entries, book.asks, true)
	entries = appendSide(entries, book.bids, false)

	return entries
}

func appendSide(entries []orderEntry, side bookSide, ask bool) []orderEntry {
	ids := make([]string, 0, len(side))

	for id := range side {
		ids = append(ids, id)
	}

	sort.Strings(ids)

	for _, id := range ids {
		entries = append(entries, orderEntry{order: side[id], ask: ask})
	}

	return entries
}

/*
midPrice is the average of the best bid and best ask currently resident in the
book. A one-sided book (no orders ever seen on the other side yet) has no
defined midpoint.
*/
func midPrice(book *symbolBook) float64 {
	bestBid := bestPrice(book.bids, true)
	bestAsk := bestPrice(book.asks, false)

	if bestBid <= 0 && bestAsk <= 0 {
		return 0
	}

	if bestBid <= 0 {
		return bestAsk
	}

	if bestAsk <= 0 {
		return bestBid
	}

	return (bestBid + bestAsk) / 2
}

/*
bestPrice finds the best resident price on one side: highest for bids, lowest
for asks. Returns 0 when the side has no usable order.
*/
func bestPrice(side bookSide, highest bool) float64 {
	best := 0.0

	for _, order := range side {
		if order.LimitPrice == nil {
			continue
		}

		price := order.LimitPrice.Float64()

		if price <= 0 {
			continue
		}

		if best == 0 || (highest && price > best) || (!highest && price < best) {
			best = price
		}
	}

	return best
}

/*
orderState projects one order into its resident oscillator State.
*/
func orderState(
	entry orderEntry,
	seq int,
	price, quantity, mid, scale float64,
	ageRank, queueRank, total int,
	symbolIndex uint32,
	grid [3]int,
) *sensorium.State {
	sidePositive := 0

	if entry.ask {
		sidePositive = 1
	}

	token := packToken(symbolIndex, sidePositive)
	x, y, z := marketPosition(price, mid, quantity, ageRank, total, grid)

	state, _ := sensorium.StatePool.Get().(*sensorium.State)

	state.Bytes[0] = int64(token)
	state.Seqs[0] = int64(seq)
	state.TokenIDs[0] = int64(token)
	state.ContentIDs[0] = int64(orderHash(entry.order))
	state.Phase[0] = orderPhase(queueRank, sidePositive)
	state.Omega[0] = orderOmega(price, mid, scale)
	state.Energy[0] = unitOscillatorEnergy
	state.Mass[0] = unitCarrierMass
	// Heat is the coupling fuel the coherence broadcast spends (Q in the
	// kernel's homeostasis budget). Particles are born with a deterministic
	// thermal store so Ψ(ω) has fuel to ignite immediately; collisions and
	// Planck relaxation then sustain and redistribute it.
	state.Heat[0] = 0.3 + 0.7*float32(orderHash(entry.order)&0xFF)/255
	state.Amp[0] = float32(math.Sqrt(float64(unitOscillatorEnergy)))
	state.Pos[0] = x
	state.Pos[1] = y
	state.Pos[2] = z
	state.Vel[0] = 0
	state.Vel[1] = 0
	state.Vel[2] = 0
	state.Clamped[0] = false
	state.Dark[0] = false

	return state
}

func packToken(symbolIndex uint32, sidePositive int) uint32 {
	sideBit := uint32(0)

	if sidePositive > 0 {
		sideBit = 1
	}

	return ((symbolIndex & symbolIndexMask) << 1) | sideBit
}

/*
orderHash mixes the order identity into a stable content fingerprint so the
content ID does not depend on map iteration order.
*/
func orderHash(order kraken.Level3Order) uint32 {
	const (
		offset = uint32(2166136261)
		prime  = uint32(16777619)
	)

	hash := offset

	for _, char := range order.OrderID {
		hash ^= uint32(char)
		hash *= prime
	}

	return hash & symbolIndexMask
}

func universeIndex(names []string, symbol string) (uint32, bool) {
	for index, name := range names {
		if name == symbol {
			return uint32(index), true
		}
	}

	return 0, false
}

/*
omegaScale folds one book snapshot into the symbol's accumulated squared
log-distance from mid and returns its root mean square — the symbol's own
measure of how far price moves to be distinguishable from resting depth.
*/
func (dataset *Dataset) omegaScale(
	symbol string,
	entries []orderEntry,
	mid float64,
) float64 {
	dataset.mutex.Lock()
	defer dataset.mutex.Unlock()

	accumulator := dataset.scales[symbol]

	if accumulator == nil {
		accumulator = &scaleAccumulator{}
		dataset.scales[symbol] = accumulator
	}

	logMid := math.Log(mid)

	for _, entry := range entries {
		price := entry.order.LimitPrice.Float64()

		if price <= 0 {
			continue
		}

		deviation := math.Log(price) - logMid
		accumulator.add(deviation * deviation)
	}

	scale := accumulator.rms()

	if scale > 0 {
		dataset.converged[symbol] = scale
	}

	return scale
}

/*
orderOmega maps log distance from mid onto the content-frequency lattice through
tanh so distance ordering survives and no particle rails the bounds. The sign
is meaningful: bids sit below mid, asks above it.
*/
func orderOmega(price, mid, scale float64) float32 {
	if !(scale > 0) || mid <= 0 || price <= 0 {
		return 0
	}

	deviation := math.Log(price) - math.Log(mid)

	return float32(math.Tanh(deviation/scale) * omegaHalfSpan)
}

/*
ageRanks ranks orders oldest-first by timestamp, ID breaking ties, so queue age
maps onto the Z axis without inheriting map iteration order.
*/
func ageRanks(entries []orderEntry) []int {
	indices := make([]int, len(entries))

	for index := range indices {
		indices[index] = index
	}

	sort.Slice(indices, func(left, right int) bool {
		first := entries[indices[left]].order
		second := entries[indices[right]].order

		if !first.Timestamp.Equal(second.Timestamp) {
			return first.Timestamp.Before(second.Timestamp)
		}

		return first.OrderID < second.OrderID
	})

	ranks := make([]int, len(entries))

	for rank, index := range indices {
		ranks[index] = rank
	}

	return ranks
}

/*
queueRanks ranks every order within its own side by exchange price-time
priority. Bids rank best-first by descending price, asks best-first by ascending
price; timestamp then ID complete the ordering.
*/
func queueRanks(entries []orderEntry) []int {
	indices := make([]int, len(entries))

	for index := range indices {
		indices[index] = index
	}

	sort.Slice(indices, func(left, right int) bool {
		first := entries[indices[left]]
		second := entries[indices[right]]

		if first.ask != second.ask {
			return !first.ask
		}

		if cmp := first.order.LimitPrice.Cmp(second.order.LimitPrice); cmp != 0 {
			if !first.ask {
				return cmp > 0
			}

			return cmp < 0
		}

		if !first.order.Timestamp.Equal(second.order.Timestamp) {
			return first.order.Timestamp.Before(second.order.Timestamp)
		}

		return first.order.OrderID < second.order.OrderID
	})

	ranks := make([]int, len(entries))
	askRank := 0
	bidRank := 0

	for _, index := range indices {
		if entries[index].ask {
			ranks[index] = askRank
			askRank++
		} else {
			ranks[index] = bidRank
			bidRank++
		}
	}

	return ranks
}

/*
marketPosition maps relative log price, log quantity, and age rank into grid
cell coordinates, x/y/z each within [0, extent).
*/
func marketPosition(
	price, mid, quantity float64,
	ageRank, total int,
	grid [3]int,
) (float32, float32, float32) {
	extent := max(grid[0], grid[1], grid[2])
	spacing := float32(1) / float32(extent)
	maxCell := float32(extent - 1)

	xUnit := unitRange(math.Log(price)-math.Log(mid), grid[0])
	yUnit := unitRange(math.Log(quantity), grid[1])
	zUnit := 0.0

	if total > 1 {
		zUnit = float64(ageRank) / float64(total-1)
	}

	return xUnit * maxCell * spacing,
		yUnit * maxCell * spacing,
		float32(zUnit) * maxCell * spacing
}

/*
unitRange maps a value into [0, extent) using the configured axis extent so a
particle always lands inside the periodic lattice.
*/
func unitRange(value float64, extent int) float32 {
	if extent <= 1 {
		return 0
	}

	half := float32(extent-1) / 2
	tanhValue := float32(math.Tanh(value))

	return half + half*tanhValue
}

/*
orderPhase derives the phase angle: bids span [0, π), asks [π, 2π), swept by the
order's price-time queue rank within its side.
*/
func orderPhase(queueRank, sidePositive int) float32 {
	progress := float64(queueRank) / 64

	base := 0.0

	if sidePositive > 0 {
		base = math.Pi
	}

	return float32(math.Mod(base+math.Pi*progress, 2*math.Pi))
}

func manifoldGrid() [3]int {
	return [3]int{
		system.Cfg.Manifold.Grid.X,
		system.Cfg.Manifold.Grid.Y,
		system.Cfg.Manifold.Grid.Z,
	}
}
