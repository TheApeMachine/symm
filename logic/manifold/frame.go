package manifold

import (
	"math"
	"sync"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Level3 arrives as one-sided incremental updates, so a message routinely carries
a single order. Standardizing that order against its own message gives a zero
deviation on every axis and places it at the origin, which is how thousands of
distinct orders end up stacked on one coordinate. The frame that places an order
therefore has to be resident, not derived from the message.

These are the retained slots for one symbol: a Standardizer over log price and
another over log quantity. Each observation is scored against the moments the
symbol has already shown and then folds itself into them, so the projector never
waits out a warmup — the first order is placed at the centre it defines, and the
frame sharpens from there.
*/
var (
	priceValue = types.MustIntern("manifold/frame/price/value")
	priceScore = types.MustIntern("manifold/frame/price/z/value")
	sizeValue  = types.MustIntern("manifold/frame/size/value")
	sizeScore  = types.MustIntern("manifold/frame/size/z/value")

	standardizePrice = adaptive.Standardizer("manifold/frame/price")
	standardizeSize  = adaptive.Standardizer("manifold/frame/size")
)

/*
frames holds one resident frame per symbol. Level3 arrives for the whole
universe across many goroutines, so the registry is guarded; the frames
themselves are only ever touched by the projector holding that lock.
*/
type frames struct {
	mu       sync.Mutex
	bySymbol map[string]*types.Frame
}

func newFrames() *frames {
	return &frames{bySymbol: make(map[string]*types.Frame)}
}

/*
place scores one order against its symbol's resident frame and returns the
position it occupies on the domain's price and quantity axes, along with the
signed price deviation the oscillator's frequency is derived from.

The domain is periodic — position_to_cell wraps a coordinate into [0, grid) — so
a raw z-score, which is unbounded and routinely several times the domain's
extent, folds back onto the faces and stacks particles into walls. Squashing the
score through tanh keeps every order strictly inside the domain while preserving
its ordering and its distance from the frame's centre, so dispersion reads as
real spatial spread rather than as wrapping.
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
		resident = &types.Frame{}
		registry.bySymbol[symbol] = resident
	}

	resident.Put(priceValue, logPrice)
	standardizePrice(resident)
	priceDeviation, _ = resident.Get(priceScore)

	if !hasQuantity {
		return unitAxis(priceDeviation), 0.5, priceDeviation, 0
	}

	resident.Put(sizeValue, logQuantity)
	standardizeSize(resident)
	quantityDeviation, _ = resident.Get(sizeScore)

	return unitAxis(priceDeviation),
		unitAxis(quantityDeviation),
		priceDeviation,
		quantityDeviation
}

/*
unitAxis squashes a signed deviation onto (0, 1), the extent of one domain axis,
placing the frame's centre at the middle of the domain. frameAxisSpan is how
many deviations span the axis before tanh saturates: beyond it orders still
order correctly, they simply crowd toward the wall they are heading for.
*/
func unitAxis(deviation float64) float64 {
	return 0.5 + 0.5*math.Tanh(deviation/frameAxisSpan)
}
