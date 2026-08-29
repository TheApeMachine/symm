/*
Package morphology measures the shape of an order book as a geometric object:
where displayed notional sits along the price axis, how symmetric the two
sides' shapes are, how concentrated each side is, and how much the whole shape
moved since the last observation.

It is deliberately a measuring instrument, never a judge. Every emitted fact
is a dimensionless, unitless description of book geometry: a distance in spread
units, a cumulative-disagreement statistic, a concentration, an entropy, and a
structural change. There is no manipulation score, no "synthetic"/"suspicious"
label, no fixed symmetry threshold, and no arbitrary depth bucket — the shape
is the book's own levels, normalized by its own current spread and each level
weighted by its own side notional.

Bilateral shape (book_shape_distance, book_shape_ks) reflects each side around
the midpoint onto a single positive distance axis, so a perfectly mirrored book
has distance zero; the sides' physical separation above/below mid is not what
bilateral morphology measures. Whole-book structural change (morphology_change)
keeps signed distance-from-mid so physical bid/ask placement remains part of
the change.

The generic distribution mathematics (Wasserstein, Kolmogorov-Smirnov,
entropy, Herfindahl) lives in nomagique/distribution, including the merged-walk
Wasserstein1Pairs / KolmogorovSmirnovPairs that compare two sorted streams with
no union, map, or combined snapshot. This package only projects a live book into
shape coordinates and calls it — and it reads the authoritative shared book only
inside its protected read callback, never letting the pointer escape.
*/
package morphology

import (
	"sort"
	"sync"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/distribution"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
Output slots the projector names into Measurement metrics.
*/
var (
	symbolShapeDistance    = nmtypes.MustIntern("morphology/book_shape_distance")
	symbolShapeKS          = nmtypes.MustIntern("morphology/book_shape_ks")
	symbolConcentrationBid = nmtypes.MustIntern("morphology/concentration_bid")
	symbolConcentrationAsk = nmtypes.MustIntern("morphology/concentration_ask")
	symbolEntropyBid       = nmtypes.MustIntern("morphology/entropy_bid")
	symbolEntropyAsk       = nmtypes.MustIntern("morphology/entropy_ask")
	symbolMorphologyChange = nmtypes.MustIntern("morphology/morphology_change")
)

/*
Book is the book-shape market entity. It reads the shared book per step and
projects one dimensionless shape Measurement. It retains the prior whole-book
shape per symbol — a single overwritten shape each, the same bounded-resident-
state contract as the shared book — so structural change is measured causally.
*/
type Book struct {
	projector *data.Projector

	mu       sync.Mutex
	previous map[string][]distribution.WeightedPoint
}

func NewBook() *Book {
	return &Book{
		previous: make(map[string][]distribution.WeightedPoint),
		projector: data.NewProjector(
			data.Binding{From: symbolShapeDistance, Name: "book_shape_distance", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolShapeKS, Name: "book_shape_ks", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolConcentrationBid, Name: "concentration:bid", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolConcentrationAsk, Name: "concentration:ask", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolEntropyBid, Name: "entropy:bid", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolEntropyAsk, Name: "entropy:ask", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolMorphologyChange, Name: "morphology_change", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		),
	}
}

func (morphology *Book) Close() error { return nil }

/*
Step projects this one Level3Data message's visible bid/ask orders into shape
facts in a single pass and emits exactly one descriptive Measurement. A
degenerate message (crossed, no spread, one empty side) yields no measurement.
*/
func (morphology *Book) Step(message kraken.Level3Data) *data.Measurement[float64] {
	if morphology == nil {
		return nil
	}

	bidFolded, askFolded, whole, ok := projectShape(message)

	if !ok {
		return nil
	}

	shapeDistance := distribution.Wasserstein1Pairs(bidFolded, askFolded)
	shapeKS := distribution.KolmogorovSmirnovPairs(bidFolded, askFolded)
	concentrationBid := distribution.ConcentrationPoints(bidFolded)
	concentrationAsk := distribution.ConcentrationPoints(askFolded)
	entropyBid := distribution.EntropyPoints(bidFolded)
	entropyAsk := distribution.EntropyPoints(askFolded)

	morphologyChange, changed := morphology.recordChange(message.Symbol, whole)

	input := nmtypes.Frame{}
	input.Put(symbolShapeDistance, shapeDistance)
	input.Put(symbolShapeKS, shapeKS)
	input.Put(symbolConcentrationBid, concentrationBid)
	input.Put(symbolConcentrationAsk, concentrationAsk)
	input.Put(symbolEntropyBid, entropyBid)
	input.Put(symbolEntropyAsk, entropyAsk)

	if changed {
		input.Put(symbolMorphologyChange, morphologyChange)
	}

	return morphology.projector.Project(message.Symbol, "morphology", message.Timestamp, message.Timestamp, input)
}

/*
recordChange stores the current whole-book shape and returns how far it moved
from the previous shape of the same symbol, on their merged position streams.
The first observation of a symbol has no prior and reports no change. Ownership
of the current slice transfers into the resident map, so no extra clone is made.
*/
func (morphology *Book) recordChange(symbol string, current []distribution.WeightedPoint) (float64, bool) {
	morphology.mu.Lock()

	previous, hadPrevious := morphology.previous[symbol]
	morphology.previous[symbol] = current
	morphology.mu.Unlock()

	if !hadPrevious {
		return 0, false
	}

	return distribution.Wasserstein1Pairs(previous, current), true
}

/*
projectShape walks one Level3Data message's visible bid/ask orders into folded
bilateral shapes and a signed whole-book shape. The bilateral shapes reflect
both sides onto the positive distance-from-mid axis (bid r = (mid−price)/spread,
ask r = (price−mid)/spread), so a perfectly mirrored book yields identical
streams. The whole-book shape retains the signed position ((price−mid)/spread)
so physical bid/ask placement stays part of structural change. ok is false on
a degenerate message — missing touch, non-positive spread, or an empty side.
*/
func projectShape(message kraken.Level3Data) ([]distribution.WeightedPoint, []distribution.WeightedPoint, []distribution.WeightedPoint, bool) {
	bidPrice, askPrice := 0.0, 0.0

	for _, order := range message.Bids {
		if order.LimitPrice == nil {
			continue
		}

		if price := order.LimitPrice.Float64(); price > bidPrice {
			bidPrice = price
		}
	}

	for _, order := range message.Asks {
		if order.LimitPrice == nil {
			continue
		}

		if price := order.LimitPrice.Float64(); askPrice == 0 || price < askPrice {
			askPrice = price
		}
	}

	if bidPrice == 0 || askPrice == 0 {
		return nil, nil, nil, false
	}

	spread := askPrice - bidPrice

	if spread <= 0 {
		return nil, nil, nil, false
	}

	midpoint := (bidPrice + askPrice) / 2

	bidFolded := make([]distribution.WeightedPoint, 0, len(message.Bids))
	askFolded := make([]distribution.WeightedPoint, 0, len(message.Asks))
	whole := make([]distribution.WeightedPoint, 0, len(message.Bids)+len(message.Asks))

	for _, order := range message.Bids {
		if order.LimitPrice == nil || order.OrderQty == nil {
			continue
		}

		price := order.LimitPrice.Float64()
		weight := price * order.OrderQty.Float64()

		if weight <= 0 {
			continue
		}

		signed := (price - midpoint) / spread

		bidFolded = append(bidFolded, distribution.WeightedPoint{Position: -signed, Weight: weight})
		whole = append(whole, distribution.WeightedPoint{Position: signed, Weight: weight})
	}

	for _, order := range message.Asks {
		if order.LimitPrice == nil || order.OrderQty == nil {
			continue
		}

		price := order.LimitPrice.Float64()
		weight := price * order.OrderQty.Float64()

		if weight <= 0 {
			continue
		}

		signed := (price - midpoint) / spread

		askFolded = append(askFolded, distribution.WeightedPoint{Position: signed, Weight: weight})
		whole = append(whole, distribution.WeightedPoint{Position: signed, Weight: weight})
	}

	if len(bidFolded) == 0 || len(askFolded) == 0 {
		return nil, nil, nil, false
	}

	sort.Slice(bidFolded, func(left, right int) bool { return bidFolded[left].Position < bidFolded[right].Position })
	sort.Slice(askFolded, func(left, right int) bool { return askFolded[left].Position < askFolded[right].Position })
	sort.Slice(whole, func(left, right int) bool { return whole[left].Position < whole[right].Position })

	return bidFolded, askFolded, whole, true
}
