package manifold

import (
	"iter"
	"math"
	"sort"
	"sync"

	mgrbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
)

/*
Dataset projects the resident Level3 order book into Sensorium States. Every
resting order becomes one oscillator carrying the structure the shared book
actually holds, not a byte-valued summary of it.

The mapping follows what the physics consumes directly:

  - Position X: relative log price log(price/mid), bids below asks.
  - Position Y: log order quantity, so liquidity spans orders of magnitude.
  - Position Z: queue age rank, oldest order at the front.
  - Mass:      one carrier unit per resting order; quantity lives in Y, not mass.
  - Heat:      zero on injection — heat is earned from collision and relaxation.
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
	workspace *runtime.Workspace
	scales    map[string]*scaleAccumulator
	converged map[string]float64
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
orderEntry couples a resting order with the side it rests on. The SDK Order
carries price and quantity but not its side, so the pair is threaded through
the projection.
*/
type orderEntry struct {
	order *mgrbook.Order
	side  mgrbook.BookDirection
}

const (
	unitCarrierMass      = float32(1)
	unitOscillatorEnergy = float32(1)
	symbolIndexMask      = uint32(0x7fff)
	omegaHalfSpan        = 4.0
)

func NewDataset(workspace *runtime.Workspace) *Dataset {
	return &Dataset{
		workspace: workspace,
		scales:    make(map[string]*scaleAccumulator),
		converged: make(map[string]float64),
	}
}

func (dataset *Dataset) Name() string { return "book" }

type bookReader interface {
	GetBooks() []string
	Get(symbol string, read func(*mgrbook.Book))
}

/*
Generate yields one State per resting order across every book the shared
manager owns, ask side first then bid side, level by level, in queue order.
*/
func (dataset *Dataset) Generate() iter.Seq[*sensorium.State] {
	return func(yield func(*sensorium.State) bool) {
		if dataset == nil || dataset.workspace == nil {
			return
		}

		shared, found := dataset.workspace.Shared("books")

		if !found {
			return
		}

		manager, ok := shared.(bookReader)

		if !ok || manager == nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"manifold: shared books value is not a bookReader",
				nil,
			))

			return
		}

		symbols := manager.GetBooks()

		for _, symbol := range symbols {
			var entries []orderEntry
			var mid float64

			manager.Get(symbol, func(book *mgrbook.Book) {
				if book == nil {
					return
				}

				mid = midPrice(book)
				if mid <= 0 {
					return
				}

				entries = sideEntries(book)
			})

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
				quantity := entry.order.Quantity.Float64()

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
sideEntries flattens one book into ask-then-bid entries, each tagged with its
side, decoupled from Go map iteration order.
*/
func sideEntries(book *mgrbook.Book) []orderEntry {
	entries := make([]orderEntry, 0, 1024)

	for _, side := range []*mgrbook.Side{book.Asks, book.Bids} {
		if side == nil {
			continue
		}

		for _, level := range side.Levels {
			if level == nil {
				continue
			}

			for _, order := range level.Queue() {
				if order == nil {
					continue
				}

				entries = append(entries, orderEntry{
					order: order,
					side:  side.Direction,
				})
			}
		}
	}

	return entries
}

func midPrice(book *mgrbook.Book) float64 {
	ask := book.BestAsk()
	bid := book.BestBid()

	if (ask == nil || ask.Price == nil) && (bid == nil || bid.Price == nil) {
		return 0
	}

	if ask == nil || ask.Price == nil {
		return bid.Price.Float64()
	}

	if bid == nil || bid.Price == nil {
		return ask.Price.Float64()
	}

	return (ask.Price.Float64() + bid.Price.Float64()) / 2
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

	if entry.side == mgrbook.Ask {
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
	state.Heat[0] = 0
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
func orderHash(order *mgrbook.Order) uint32 {
	const (
		offset = uint32(2166136261)
		prime  = uint32(16777619)
	)

	hash := offset

	for _, char := range order.ID {
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

		return first.ID < second.ID
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

		if first.side != second.side {
			return first.side == mgrbook.Bid
		}

		if cmp := first.order.LimitPrice.Cmp(second.order.LimitPrice); cmp != 0 {
			if first.side == mgrbook.Bid {
				return cmp > 0
			}

			return cmp < 0
		}

		if !first.order.Timestamp.Equal(second.order.Timestamp) {
			return first.order.Timestamp.Before(second.order.Timestamp)
		}

		return first.order.ID < second.order.ID
	})

	ranks := make([]int, len(entries))
	askRank := 0
	bidRank := 0

	for _, index := range indices {
		side := entries[index].side
		if side == mgrbook.Ask {
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
