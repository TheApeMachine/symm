package manifold

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
)

/*
Dataset projects one Level3 message's resting orders into Sensorium States.

It holds no book and no retained price. Kraken sends Level-3 as one-sided
incremental updates, so any projection that needs BOTH sides of a touch before
it can place a particle simply never places one: the two sides are never
present in the same message. The previous implementation derived a two-sided
midpoint for exactly that purpose and, as a result, emitted nothing at all.

Nothing here needs an absolute price anchor. The physics domain is a periodic
torus — position_to_cell wraps a particle into [0, grid) — so what determines a
particle's cell is its position RELATIVE to the other particles, not its
distance from some global reference. The message's own orders therefore supply
their own frame:

  - Position X: log price spread about this message's own log-price mean, so
    the axis is the price dispersion the message actually carries.
  - Position Y: log quantity, likewise centered, so liquidity spanning orders
    of magnitude occupies the axis rather than collapsing into one cell.
  - Position Z: queue rank, oldest order at the front.
  - Mass:       one carrier unit per resting order; size lives in Y, not mass.
  - Energy:     one unit per order, lifted by the Hawkes excitation on the side
    aggressive flow is actually hitting.
  - Amp:        sqrt(Energy), the wave amplitude the observers render.
  - Phase:      bids in [0, π), asks in [π, 2π) — the spread as a quantum
    boundary — swept by queue priority within each side.
  - Omega:      the order's own signed log-price deviation, scaled by the
    message's dispersion, so an order's frequency reflects how far out of line
    it sits rather than an arbitrary constant.
  - Heat:       a deterministic per-order thermal store, the coupling fuel the
    coherence broadcast spends.

A message carrying a single usable order has no dispersion to speak of; it
still projects, at the center of the price and quantity axes, because one
resting order is a real observation even though it spans nothing.
*/
type Dataset struct {
}

const (
	unitCarrierMass      = float32(1)
	unitOscillatorEnergy = float32(1)
	symbolIndexMask      = uint32(0x7fff)
	omegaHalfSpan        = 4.0

	// dispersionFloor keeps a degenerate message — every order at one price,
	// or a single order — from dividing by zero when it is standardized. It is
	// a floor on an observed standard deviation in log space, not a magic
	// width: below it the axis is genuinely flat and every particle belongs in
	// the same place.
	dispersionFloor = 1e-9
)

/*
NewDataset constructs a streaming projector. It retains nothing: every message
is projected forward exactly once from its own contents.
*/
func NewDataset() *Dataset {
	return &Dataset{}
}

func (dataset *Dataset) Name() string { return "book" }

/*
Step projects this one Level3 message's resting orders into States and yields
them. A delete removes its order from the book, so it describes no resting
particle and is skipped; bids and asks are consumed directly without a
flattened intermediary representation.
*/
func (dataset *Dataset) Step(
	message kraken.Level3Data,
	forcing forcingState,
) iter.Seq[*sensorium.State] {
	return func(yield func(*sensorium.State) bool) {
		if dataset == nil || message.Symbol == "" {
			return
		}

		orderCount, priceMean, priceDispersion,
			quantityMean, quantityDispersion := logMoments(message)

		if orderCount == 0 {
			return
		}

		symbolIndex := symbolToken(message.Symbol)

		for sidePositive, orders := range [][]kraken.Level3Order{
			message.Bids, message.Asks,
		} {
			total := 0

			for _, order := range orders {
				if usableOrder(order) {
					total++
				}
			}

			rank := 0

			for _, order := range orders {
				if !usableOrder(order) {
					continue
				}

				token := packToken(symbolIndex, sidePositive)
				priceDeviation := standardize(
					math.Log(order.LimitPrice.Float64()),
					priceMean,
					priceDispersion,
				)
				quantityDeviation := standardize(
					math.Log(order.OrderQty.Float64()),
					quantityMean,
					quantityDispersion,
				)
				contentID := orderHash(order)

				state, _ := sensorium.StatePool.Get().(*sensorium.State)

				// The Hawkes excitation fraction is the forcing amplitude above the unit
				// baseline: aggressive buy arrivals interact with resting asks, aggressive
				// sell arrivals with resting bids. No forcing observed yet leaves the
				// unit baseline.
				energy := unitOscillatorEnergy + forcing.sellExcitation

				if sidePositive > 0 {
					energy = unitOscillatorEnergy + forcing.buyExcitation
				}

				state.N = 1
				state.Bytes[0] = int64(token)
				state.Seqs[0] = int64(rank)
				state.TokenIDs[0] = int64(token)
				state.ContentIDs[0] = int64(contentID)
				state.Phase[0] = orderPhase(rank, total, sidePositive)
				state.Omega[0] = float32(math.Tanh(priceDeviation) * omegaHalfSpan)
				state.Energy[0] = energy
				state.Mass[0] = unitCarrierMass
				state.Heat[0] = 0.3 + 0.7*float32(contentID&0xFF)/255
				state.Amp[0] = float32(math.Sqrt(float64(energy)))
				state.Pos[0] = float32(priceDeviation)
				state.Pos[1] = float32(quantityDeviation)
				state.Pos[2] = queueDepth(rank, total)
				state.Vel[0] = 0
				state.Vel[1] = 0
				state.Vel[2] = 0
				state.Clamped[0] = false
				state.Dark[0] = false
				rank++

				if !yield(state) {
					return
				}
			}
		}
	}
}

/*
usableOrder reports whether an order describes a resting particle: it must
still be on the book after this message, and it must carry a positive price
and size for the log-space projection to be defined.
*/
func usableOrder(order kraken.Level3Order) bool {
	return order.Resting() &&
		order.LimitPrice.Float64() > 0 &&
		order.OrderQty.Float64() > 0
}

/*
logMoments reduces one message's orders to the mean and standard deviation of
price and quantity in log space. This is the frame the message supplies for
itself: no retained state, no imposed constant, and no reference price the feed
never sends in one piece.
*/
func logMoments(
	message kraken.Level3Data,
) (
	orderCount int,
	priceMean, priceDispersion float64,
	quantityMean, quantityDispersion float64,
) {
	for _, orders := range [][]kraken.Level3Order{message.Bids, message.Asks} {
		for _, order := range orders {
			if !usableOrder(order) {
				continue
			}

			orderCount++
			priceMean += math.Log(order.LimitPrice.Float64())
			quantityMean += math.Log(order.OrderQty.Float64())
		}
	}

	if orderCount == 0 {
		return
	}

	priceMean /= float64(orderCount)
	quantityMean /= float64(orderCount)

	for _, orders := range [][]kraken.Level3Order{message.Bids, message.Asks} {
		for _, order := range orders {
			if !usableOrder(order) {
				continue
			}

			priceDeviation := math.Log(order.LimitPrice.Float64()) - priceMean
			quantityDeviation := math.Log(order.OrderQty.Float64()) - quantityMean
			priceDispersion += priceDeviation * priceDeviation
			quantityDispersion += quantityDeviation * quantityDeviation
		}
	}

	priceDispersion = math.Sqrt(priceDispersion / float64(orderCount))
	quantityDispersion = math.Sqrt(quantityDispersion / float64(orderCount))

	return
}

/*
standardize expresses one log observation as a signed distance from its
message's own log mean, in units of that message's own dispersion. A message
with no dispersion places every particle at the center of the axis, which is
where a set of identical observations genuinely belongs.
*/
func standardize(value, mean, dispersion float64) float64 {
	if dispersion < dispersionFloor {
		return 0
	}

	return (value - mean) / dispersion
}

/*
queueDepth maps queue rank onto the third axis, oldest order at the front. A
single-order message has no queue to express and sits at the front.
*/
func queueDepth(rank, total int) float32 {
	if total <= 1 {
		return 0
	}

	return float32(rank) / float32(total-1)
}

/*
orderPhase sweeps queue priority across the side's half of the phase circle:
bids occupy [0, π) and asks [π, 2π), so the spread is a quantum boundary and
an order's phase encodes where it sits in its own side's queue.
*/
func orderPhase(rank, total, sidePositive int) float32 {
	position := 0.0

	if total > 1 {
		position = float64(rank) / float64(total)
	}

	return float32(math.Pi * (float64(sidePositive) + position))
}

func packToken(symbolIndex uint32, sidePositive int) uint32 {
	sideBit := uint32(0)

	if sidePositive > 0 {
		sideBit = 1
	}

	return ((symbolIndex & symbolIndexMask) << 1) | sideBit
}

/*
symbolToken derives a stable particle-token identity for a symbol from the
symbol itself, so no shared counter is needed to keep one symbol's token space
from colliding with another's.
*/
func symbolToken(symbol string) uint32 {
	const (
		offset = uint32(2166136261)
		prime  = uint32(16777619)
	)

	hash := offset

	for index := 0; index < len(symbol); index++ {
		hash ^= uint32(symbol[index])
		hash *= prime
	}

	return hash & symbolIndexMask
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

	for index := 0; index < len(order.OrderID); index++ {
		hash ^= uint32(order.OrderID[index])
		hash *= prime
	}

	return hash
}
