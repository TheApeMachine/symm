package manifold

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/system"
)

/*
Dataset projects one Level3 message's resting orders into Sensorium States. It
holds no book: every message is projected forward exactly once, and the only
state retained is the composed touch (running best-bid / best-ask, from which
the center is derived) and the symbol's own running scale — both held by a
nomagique Number keyed by symbol, not by a reconstructed order book.

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
	// touch tracks the center per symbol: the running best price on each side,
	// reduced to a midpoint, fed one message at a time.
	touch *TouchCenter
}

/*
orderEntry couples one resting order with the side it rests on.
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

/*
NewDataset constructs an empty streaming projector. It retains no book state;
the touch composer initializes lazily per symbol on first message.
*/
func NewDataset() *Dataset {
	return &Dataset{
		touch: newTouchCenter(),
	}
}

func (dataset *Dataset) Name() string { return "book" }

/*
Step projects this one Level3 message's visible orders into States and yields
them. Nothing accumulates: the message is walked once, its best bid/ask feed the
touch composer for the symbol's center, and its orders become the projected
output.
*/
func (dataset *Dataset) Step(message kraken.Level3Data) iter.Seq[*sensorium.State] {
	return func(yield func(*sensorium.State) bool) {
		if dataset == nil || message.Symbol == "" {
			return
		}

		bestBid, bestAsk, mid, scale := dataset.projectSymmetry(message)

		_ = bestBid
		_ = bestAsk

		if mid <= 0 {
			return
		}

		entries := dataset.entries(message)
		grid := manifoldGrid()
		symbolIndex := dataset.symbolIndex(message.Symbol)

		for seq, entry := range entries {
			price := entry.order.LimitPrice.Float64()
			quantity := entry.order.OrderQty.Float64()

			if price <= 0 || quantity <= 0 {
				continue
			}

			state := dataset.orderState(
				entry, seq, len(entries),
				price, quantity, mid, scale,
				symbolIndex, grid,
			)

			if !yield(state) {
				return
			}
		}
	}
}

/*
entries flattens this one message's orders into ask-then-bid entries, tagged
with their side, in arrival order. It does not sort and does not consult any
retained book.
*/
func (dataset *Dataset) entries(message kraken.Level3Data) []orderEntry {
	entries := make([]orderEntry, 0, len(message.Asks)+len(message.Bids))

	for _, order := range message.Asks {
		if order.LimitPrice == nil || order.OrderQty == nil {
			continue
		}

		entries = append(entries, orderEntry{order: order, ask: true})
	}

	for _, order := range message.Bids {
		if order.LimitPrice == nil || order.OrderQty == nil {
			continue
		}

		entries = append(entries, orderEntry{order: order, ask: false})
	}

	return entries
}

/*
projectSymmetry folds this one message into the symbol's touch composer and
returns the best bid, best ask, the derived midpoint, and the symbol's running
scale. bestBid is the highest bid in the message merged with the retained
running best; bestAsk is the lowest ask likewise.
*/
func (dataset *Dataset) projectSymmetry(
	message kraken.Level3Data,
) (bestBid, bestAsk, mid, scale float64) {
	bestBid = 0
	bestAsk = 0

	for _, order := range message.Bids {
		if order.LimitPrice == nil {
			continue
		}

		if price := order.LimitPrice.Float64(); price > bestBid {
			bestBid = price
		}
	}

	for _, order := range message.Asks {
		if order.LimitPrice == nil {
			continue
		}

		if price := order.LimitPrice.Float64(); price > 0 &&
			(bestAsk == 0 || price < bestAsk) {
			bestAsk = price
		}
	}

	mid = dataset.touch.Observe(
		message.Symbol,
		bestBid,
		bestAsk,
		float64(message.Timestamp.Unix()),
	)
	scale = dataset.touch.Scale(message.Symbol, message, mid)

	return bestBid, bestAsk, mid, scale
}

func (dataset *Dataset) symbolIndex(symbol string) uint32 {
	return dataset.touch.Index(symbol)
}

/*
orderState projects one order into its resident oscillator State.
*/
func (dataset *Dataset) orderState(
	entry orderEntry,
	seq, total int,
	price, quantity, mid, scale float64,
	symbolIndex uint32,
	grid [3]int,
) *sensorium.State {
	sidePositive := 0

	if entry.ask {
		sidePositive = 1
	}

	token := packToken(symbolIndex, sidePositive)
	x, y, z := marketPosition(price, mid, quantity, seq, total, grid)

	state, _ := sensorium.StatePool.Get().(*sensorium.State)

	state.Bytes[0] = int64(token)
	state.Seqs[0] = int64(seq)
	state.TokenIDs[0] = int64(token)
	state.ContentIDs[0] = int64(orderHash(entry.order))
	state.Phase[0] = orderPhase(seq, sidePositive)
	state.Omega[0] = orderOmega(price, mid, scale)
	state.Energy[0] = unitOscillatorEnergy
	state.Mass[0] = unitCarrierMass
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
order's arrival rank within its side.
*/
func orderPhase(rank, sidePositive int) float32 {
	progress := float64(rank) / 64

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
