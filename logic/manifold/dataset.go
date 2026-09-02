package manifold

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
)

/*
Dataset projects one Level3 message's resting orders into Sensorium States.

It holds no book — the resident domain is the book — but it does hold a
coordinate frame per symbol. Kraken sends Level-3 as one-sided incremental
updates, so any projection that needs BOTH sides of a touch before it can place
a particle simply never places one: the two sides are never present in the same
message. Nor can a message supply its own frame: an update carrying a single
order has no dispersion, so standardizing that order against its own message
places it at the frame's origin, and thousands of distinct orders end up
stacked on one coordinate.

The frame is therefore resident per symbol (see frame.go). Each observation
folds into a running log-price and log-quantity moment, and an order is placed
against everything that symbol has shown so far:

  - Position X: log price against the symbol's resident price frame, squashed
    into the unit cube the domain spans.
  - Position Y: log quantity likewise, so liquidity spanning orders of
    magnitude occupies the axis rather than collapsing into one cell.
  - Position Z: queue rank, oldest order at the front; an update that states no
    queue spreads its order by identity rather than pinning it to the front.
  - Mass:       one carrier unit per resting order; size lives in Y, not mass.
  - Energy:     one unit per order, lifted by the Hawkes excitation on the side
    aggressive flow is actually hitting.
  - Amp:        sqrt(Energy), the wave amplitude the observers render.
  - Phase:      bids in [0, π), asks in [π, 2π) — the spread as a quantum
    boundary — swept by queue priority within each side.
  - Omega:      the order's signed price deviation in its resident frame, so an
    order's frequency reflects how far out of line it sits.
  - Heat:       a deterministic per-order thermal store, the coupling fuel the
    coherence broadcast spends.
*/
type Dataset struct {
	frames *frames
}

const (
	// unitProbeEnergy is the energy of a crystallization probe: a candidate
	// price the book never stated an order for, so it has no size to scale by
	// and enters as a light neutral test particle.
	unitProbeEnergy = float32(1)
	symbolIndexMask = uint32(0x7fff)
	omegaHalfSpan   = 4.0

	// energyFloor is the energy of the smallest order the frame has seen. It is
	// not a magic minimum: it is what keeps a dust order a real, if light,
	// participant rather than a massless one the density field cannot see and
	// the pilot wave cannot steer.
	energyFloor = 0.25

	// energySizeSpan is how much energy the frame's own size dispersion is
	// worth, so the largest orders a symbol shows carry energyFloor+energySizeSpan
	// and the smallest carry energyFloor.
	energySizeSpan = 1.75

	// frameAxisSpan is how many standard deviations of the resident frame span
	// the domain axis before tanh saturates. Beyond it orders still order
	// correctly, they simply crowd toward the wall they are heading for.
	frameAxisSpan = 3.0
)

/*
NewDataset constructs a streaming projector. It retains nothing: every message
is projected forward exactly once from its own contents.
*/
func NewDataset() *Dataset {
	return &Dataset{frames: newFrames()}
}

func (dataset *Dataset) Name() string { return "book" }

/*
Step projects this one Level3 message's resting orders into States and yields
them unclamped: every particle is free to evolve under the resident field. A
delete removes its order from the book, so it describes no resting particle
and is skipped; bids and asks are consumed directly without a flattened
intermediary representation.
*/
func (dataset *Dataset) Step(
	message kraken.Level3Data,
	forcing forcingState,
) iter.Seq[*sensorium.State] {
	return dataset.step(message, forcing, false)
}

/*
StepClamped is the BVP form of Step: observed L3 resting orders are projected
as clamped boundary particles so a relaxation pass can hold them fixed while
injected dark probe particles crystallize around them.
*/
func (dataset *Dataset) StepClamped(
	message kraken.Level3Data,
	forcing forcingState,
) iter.Seq[*sensorium.State] {
	return dataset.step(message, forcing, true)
}

func (dataset *Dataset) step(
	message kraken.Level3Data,
	forcing forcingState,
	clamped bool,
) iter.Seq[*sensorium.State] {
	return func(yield func(*sensorium.State) bool) {
		if dataset == nil || message.Symbol == "" {
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
				positionX, positionY, priceDeviation, quantityDeviation :=
					dataset.frames.place(
						message.Symbol,
						math.Log(order.LimitPrice.Float64()),
						math.Log(order.OrderQty.Float64()),
					)
				contentID := orderContentID(orderIdentity{
					symbol:  message.Symbol,
					orderID: order.OrderID,
				})

				state, _ := sensorium.StatePool.Get().(*sensorium.State)

				// The order's own size is what it brings to the field, lifted by
				// the Hawkes excitation on the side aggressive flow is actually
				// hitting: a large order in an excited regime is the strongest
				// driver. Size enters in log space because book quantities span
				// orders of magnitude, and the resident frame's own dispersion
				// is what makes "large" mean large for this symbol.
				excitation := forcing.sellExcitation

				if sidePositive > 0 {
					excitation = forcing.buyExcitation
				}

				energy := orderEnergy(quantityDeviation, excitation)

				state.N = 1
				state.Bytes[0] = int64(token)
				state.Seqs[0] = int64(rank)
				state.TokenIDs[0] = int64(token)
				state.ContentIDs[0] = contentID
				state.Phase[0] = orderPhase(rank, total, sidePositive)
				state.Omega[0] = float32(math.Tanh(priceDeviation) * omegaHalfSpan)
				state.Energy[0] = energy
				// Mass tracks energy, as the domain's own initializer defines a
				// well-formed particle. Mass is what the particle deposits as
				// density, what scales the heat it gathers back from the gas,
				// and what divides its pilot-wave guidance — so a unit constant
				// made the field a map of order COUNT and steered every order
				// identically regardless of size.
				state.Mass[0] = energy
				// Heat is the metabolic budget that pays for coupling to the
				// coherence field. It is earned from the gas in planckExchange,
				// never invented here: seeding it from the order's identity
				// handed each order an arbitrary work allowance.
				state.Heat[0] = 0
				state.Amp[0] = float32(math.Sqrt(float64(energy)))
				state.Pos[0] = float32(positionX)
				state.Pos[1] = float32(positionY)
				state.Pos[2] = queueDepth(rank, total, uint32(contentID))
				state.Vel[0] = 0
				state.Vel[1] = 0
				state.Vel[2] = 0
				state.Clamped[0] = clamped
				state.Dark[0] = false
				rank++

				// An order only becomes a particle when every field it carries
				// is determined: energy, mass and amplitude must be positive
				// and finite, and phase, omega and every coordinate must be
				// finite. A zero or non-finite seed poisons the gas, planck
				// exchange and coherence kernels the same way missing data
				// would, so the projector drops the order instead of emitting
				// an invalid particle.
				if !validParticle(state) {
					sensorium.StatePool.Put(state)
					continue
				}

				if !yield(state) {
					return
				}
			}
		}
	}
}

/*
orderEnergy is what one resting order brings to the field.

The order's size in its symbol's resident frame sets the scale and the Hawkes
excitation on its side lifts it, so a large order in an excited regime is the
strongest driver. The deviation is squashed through the logistic so a size the
frame has never seen cannot hand one order unbounded mass: the domain treats
mass as density, as the divisor of pilot-wave guidance, and as the scale of the
heat a particle gathers, and an unbounded value there is a vacuum or a
singularity in three different kernels.

The result is strictly positive. A zero-mass particle deposits no density and
divides its own guidance by the pilot wave's mass floor, and planckExchange
refuses a non-positive energy outright.
*/
func orderEnergy(quantityDeviation float64, excitation float32) float32 {
	size := 1 / (1 + math.Exp(-quantityDeviation/frameAxisSpan))

	return float32(size*energySizeSpan+energyFloor) * (1 + excitation)
}

/*
usableOrder reports whether an order describes a resting particle: it must
still be on the book after this message, and it must carry a finite positive
price and size for the log-space projection to be defined.
*/
func usableOrder(order kraken.Level3Order) bool {
	return order.Resting() && validPositive(order.LimitPrice.Float64()) &&
		validPositive(order.OrderQty.Float64())
}

/*
validPositive reports whether a value is strictly positive and finite.
*/
func validPositive(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

/*
validParticle reports whether a projected particle carries only determined
values: energy, mass and amplitude are strictly positive and finite (the
kernels divide by them and refuse a non-positive energy), and every coordinate
and oscillator field is finite. It is the data-arrival gate — an order whose
projection is missing any of these values never becomes a particle.
*/
func validParticle(state *sensorium.State) bool {
	if state == nil || state.N != 1 {
		return false
	}

	if !validPositive(float64(state.Energy[0])) ||
		!validPositive(float64(state.Mass[0])) ||
		!validPositive(float64(state.Amp[0])) {
		return false
	}

	if !isFiniteFloat32(state.Phase[0]) ||
		!isFiniteFloat32(state.Omega[0]) ||
		!isFiniteFloat32(state.Pos[0]) ||
		!isFiniteFloat32(state.Pos[1]) ||
		!isFiniteFloat32(state.Pos[2]) ||
		!isFiniteFloat32(state.Vel[0]) ||
		!isFiniteFloat32(state.Vel[1]) ||
		!isFiniteFloat32(state.Vel[2]) ||
		!isFiniteFloat32(state.Heat[0]) {
		return false
	}

	return true
}

func isFiniteFloat32(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}

/*
queueDepth maps queue rank onto the third axis, oldest order at the front.
*/
func queueDepth(rank, total int, contentID uint32) float32 {
	if total > 1 {
		return float32(rank) / float32(total-1)
	}

	// An incremental update carries one order and so expresses no queue. Its
	// depth is unknown, not zero: collapsing every such order onto the front
	// plane stacks thousands of distinct orders on one coordinate. The order's
	// own stable identity spreads them across the axis instead, which keeps
	// them distinct without inventing a queue position the message never
	// stated.
	return float32(contentID&0xFFFF) / 65535
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
