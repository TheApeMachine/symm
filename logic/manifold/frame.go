package manifold

import (
	"sync"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/equation"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
Level3 arrives as one-sided incremental updates, so a message routinely carries
a single order. Standardizing that order against its own message gives a zero
deviation on every axis and places it at the origin, which is how thousands of
distinct orders end up stacked on one coordinate. The frame that places an order
therefore has to be resident, not derived from the message.

frame is one symbol's resident coordinate system: a standardizer over log price
and another over log quantity, each composed with the squash that places its
deviation on a domain axis. Every observation is scored against the moments the
symbol has already shown and then folds itself into them, so the projector never
waits out a warmup — the first order is placed at the centre it defines, and the
frame sharpens from there.
*/
type frame struct {
	price    equation.CausalResidual
	quantity equation.CausalResidual

	priceAxis    nomagique.Pipeline
	quantityAxis nomagique.Pipeline
}

/*
newFrame composes one symbol's two axes.

Each axis is a Chain: CausalResidual scores the observation against the
moments the symbol showed BEFORE it — a standardizer that folded the current
observation into its own scale would read a burst of near-identical prices as
near-zero dispersion and blow the score up — and UnitAxis squashes that
deviation onto the domain.

The domain is periodic — position_to_cell wraps a coordinate into [0, grid) —
so a raw z-score, which is unbounded and routinely several times the domain's
extent, folds back onto the faces and stacks particles into walls. The squash
keeps every order strictly inside the domain while preserving its ordering and
its distance from the centre, so dispersion reads as real spatial spread rather
than as wrapping.
*/
func newFrame() *frame {
	built := &frame{}

	span := calculus.Constant{Value: frameAxisSpan}

	built.priceAxis = *nomagique.Number(&nomagique.Chain{
		A: &built.price,
		B: &calculus.UnitAxis{Span: span},
	})

	built.quantityAxis = *nomagique.Number(&nomagique.Chain{
		A: &built.quantity,
		B: &calculus.UnitAxis{Span: span},
	})

	return built
}

/*
frames holds one resident frame per symbol. Level3 arrives for the whole
universe across many goroutines, so the registry is guarded; the frames
themselves are only ever touched by the projector holding that lock.
*/
type frames struct {
	mu       sync.Mutex
	bySymbol map[string]*frame
}

func newFrames() *frames {
	return &frames{bySymbol: make(map[string]*frame)}
}

/*
place scores one order against its symbol's resident frame and returns the
position it occupies on the domain's price and quantity axes, along with the
signed price deviation the oscillator's frequency is derived from.
*/
func (registry *frames) place(
	symbol string,
	logPrice, logQuantity float64,
) (positionX, positionY, priceDeviation, quantityDeviation float64) {
	return registry.score(symbol, logPrice, logQuantity, true)
}

/*
placePrice scores a level that carries no size of its own — a crystallization
probe is a price the book has not stated an order for. It is placed on the price
axis by the resident frame and sits at the centre of the size axis, which is the
frame's own reading of a size it was never told.
*/
func (registry *frames) placePrice(
	symbol string,
	logPrice float64,
) (positionX, priceDeviation float64) {
	x, _, deviation, _ := registry.score(symbol, logPrice, 0, false)

	return x, deviation
}

func (registry *frames) score(
	symbol string,
	logPrice, logQuantity float64,
	hasQuantity bool,
) (positionX, positionY, priceDeviation, quantityDeviation float64) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	resident, seen := registry.bySymbol[symbol]

	if !seen {
		resident = newFrame()
		registry.bySymbol[symbol] = resident
	}

	// Each axis steps its standardizer exactly once; the deviation behind the
	// placement is read back from that same step, never re-derived.
	positionX = float64(resident.priceAxis.Step(nmtypes.Number(logPrice)))
	priceDeviation = float64(resident.price.ZScore())

	if !hasQuantity {
		return positionX, 0.5, priceDeviation, 0
	}

	positionY = float64(resident.quantityAxis.Step(nmtypes.Number(logQuantity)))
	quantityDeviation = float64(resident.quantity.ZScore())

	return positionX, positionY, priceDeviation, quantityDeviation
}
