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
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/distribution"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
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
The structural-change estimator. morphology_change is this signal's headline
metric, and it is the only emitted fact with a history to speak of: the other
facts describe the shape standing right now, while the change is a step-to-step
distance. Its causal baseline and dispersion are what let a measurement report
how far the current move stands from this symbol's own recent structural churn,
which is the quality verdict data.Measurement.Finalize turns into the SNR.
*/
const prefixChange = "morphology/change"

var changeSeries = temporal.NewSeries(prefixChange)

/*
Book is the book-shape market entity. It reads the shared book per step and
projects one dimensionless shape Measurement. It retains the prior whole-book
shape per symbol — a single overwritten shape each, the same bounded-resident-
state contract as the shared book — so structural change is measured causally.
*/
type Book struct {
	number    *nomagique.Number[string]
	projector *data.Projector

	mu         sync.Mutex
	previous   map[string][]distribution.WeightedPoint
	lastBid    map[string]float64
	lastAsk    map[string]float64
	lastBidRaw map[string][]distribution.WeightedPoint
	lastAskRaw map[string][]distribution.WeightedPoint
}

func NewBook() *Book {
	return &Book{
		previous:   make(map[string][]distribution.WeightedPoint),
		lastBid:    make(map[string]float64),
		lastAsk:    make(map[string]float64),
		lastBidRaw: make(map[string][]distribution.WeightedPoint),
		lastAskRaw: make(map[string][]distribution.WeightedPoint),
		number: nomagique.NewNumber[string](nmtypes.Pipe(
			// The causal estimator over structural change: evaluate this step's
			// distance against the baseline built strictly from previous steps,
			// then let the baseline adapt and the window retain the observation.
			temporal.Window(prefixChange),
			statistic.ZScore(prefixChange),
			statistic.Baseline(prefixChange),
			statistic.QualityFrom(prefixChange),
		)),
		projector: data.NewProjector(
			data.Binding{From: symbolShapeDistance, Name: "book_shape_distance", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolShapeKS, Name: "book_shape_ks", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolConcentrationBid, Name: "concentration:bid", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolConcentrationAsk, Name: "concentration:ask", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolEntropyBid, Name: "entropy:bid", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolEntropyAsk, Name: "entropy:ask", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolMorphologyChange, Name: "morphology_change", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmtypes.MustIntern(temporal.JoinPrefix(prefixChange, "baseline/value")), Name: "morphology_change_baseline", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmtypes.MustIntern(temporal.JoinPrefix(prefixChange, "z/value")), Name: "morphology_change_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
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

	bidFolded, askFolded, whole, ok := morphology.projectShapeWithCache(message)

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

	// Only a step that actually had a prior shape carries a structural change,
	// and only then is there anything for the estimator to measure. Without it
	// the shape facts still project; the measurement simply reports no SNR.
	if !changed {
		return morphology.projector.Project(message.Symbol, "morphology", message.Timestamp, message.Timestamp, input)
	}

	if committed, found := morphology.number.Project(message.Symbol); found {
		previousSec, _ := committed.Get(changeSeries.SecSymbol)
		previousNsec, _ := committed.Get(changeSeries.NsecSymbol)

		if float64(message.Timestamp.Unix()) < previousSec ||
			(float64(message.Timestamp.Unix()) == previousSec && float64(message.Timestamp.Nanosecond()) < previousNsec) {
			return nil
		}
	}

	input.Put(symbolMorphologyChange, morphologyChange)
	input.Put(changeSeries.ValueSymbol, morphologyChange)
	input.Put(changeSeries.SecSymbol, float64(message.Timestamp.Unix()))
	input.Put(changeSeries.NsecSymbol, float64(message.Timestamp.Nanosecond()))


	return morphology.projector.Project(
		message.Symbol,
		"morphology",
		message.Timestamp,
		message.Timestamp,
		morphology.number.Step(message.Symbol, input),
	)
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
projectShapeWithCache projects one Level3Data message as projectShape does,
but borrows each side's last observed raw orders when the message is one-sided
(Kraken sends Level-3 as 1-sided incremental updates), re-folding the borrowed
side against the current touch so the shape reflects the resting book assumed
unchanged on the absent side. See projectShape.
*/
func (morphology *Book) projectShapeWithCache(message kraken.Level3Data) ([]distribution.WeightedPoint, []distribution.WeightedPoint, []distribution.WeightedPoint, bool) {
	morphology.mu.Lock()

	bidPrice := morphology.lastBid[message.Symbol]
	askPrice := morphology.lastAsk[message.Symbol]

	bidRaw := rawSide(message.Bids)

	for _, point := range bidRaw {
		if point.Position > bidPrice {
			bidPrice = point.Position
		}
	}

	askRaw := rawSide(message.Asks)

	for _, point := range askRaw {
		if askPrice == 0 || point.Position < askPrice {
			askPrice = point.Position
		}
	}

	if len(bidRaw) == 0 {
		bidRaw = morphology.lastBidRaw[message.Symbol]
	}

	if len(askRaw) == 0 {
		askRaw = morphology.lastAskRaw[message.Symbol]
	}

	if bidPrice > 0 {
		morphology.lastBid[message.Symbol] = bidPrice

		if len(bidRaw) > 0 {
			morphology.lastBidRaw[message.Symbol] = bidRaw
		}
	}

	if askPrice > 0 {
		morphology.lastAsk[message.Symbol] = askPrice

		if len(askRaw) > 0 {
			morphology.lastAskRaw[message.Symbol] = askRaw
		}
	}

	morphology.mu.Unlock()

	if bidPrice == 0 || askPrice == 0 || askPrice <= bidPrice {
		return nil, nil, nil, false
	}

	return foldRawSides(bidRaw, askRaw, bidPrice, askPrice)
}

/*
rawSide projects a side's usable orders onto raw price/notional points, in
order of appearance. Orders without a price, without a quantity, or with
non-positive notional are skipped.
*/
func rawSide(orders []kraken.Level3Order) []distribution.WeightedPoint {
	points := make([]distribution.WeightedPoint, 0, len(orders))

	for _, order := range orders {
		if !order.Resting() {
			continue
		}

		price := order.LimitPrice.Float64()
		weight := price * order.OrderQty.Float64()

		if weight <= 0 {
			continue
		}

		points = append(points, distribution.WeightedPoint{Position: price, Weight: weight})
	}

	return points
}

/*
foldRawSides folds raw price/notional points from both sides onto the bilateral
and whole-book shape coordinates for a known uncrossed touch, reusing the same
normalization as foldShape. ok is false when either side has no usable points.
*/
func foldRawSides(bidRaw []distribution.WeightedPoint, askRaw []distribution.WeightedPoint, bidPrice float64, askPrice float64) ([]distribution.WeightedPoint, []distribution.WeightedPoint, []distribution.WeightedPoint, bool) {
	if len(bidRaw) == 0 || len(askRaw) == 0 {
		return nil, nil, nil, false
	}

	spread := askPrice - bidPrice
	midpoint := (bidPrice + askPrice) / 2

	bidFolded := make([]distribution.WeightedPoint, 0, len(bidRaw))
	askFolded := make([]distribution.WeightedPoint, 0, len(askRaw))
	whole := make([]distribution.WeightedPoint, 0, len(bidRaw)+len(askRaw))

	for _, point := range bidRaw {
		signed := (point.Position - midpoint) / spread
		bidFolded = append(bidFolded, distribution.WeightedPoint{Position: -signed, Weight: point.Weight})
		whole = append(whole, distribution.WeightedPoint{Position: signed, Weight: point.Weight})
	}

	for _, point := range askRaw {
		signed := (point.Position - midpoint) / spread
		askFolded = append(askFolded, distribution.WeightedPoint{Position: signed, Weight: point.Weight})
		whole = append(whole, distribution.WeightedPoint{Position: signed, Weight: point.Weight})
	}

	sort.Slice(bidFolded, func(left, right int) bool { return bidFolded[left].Position < bidFolded[right].Position })
	sort.Slice(askFolded, func(left, right int) bool { return askFolded[left].Position < askFolded[right].Position })
	sort.Slice(whole, func(left, right int) bool { return whole[left].Position < whole[right].Position })

	return bidFolded, askFolded, whole, true
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
		if !order.Resting() {
			continue
		}

		if price := order.LimitPrice.Float64(); price > bidPrice {
			bidPrice = price
		}
	}

	for _, order := range message.Asks {
		if !order.Resting() {
			continue
		}

		if price := order.LimitPrice.Float64(); askPrice == 0 || price < askPrice {
			askPrice = price
		}
	}

	if bidPrice == 0 || askPrice == 0 || askPrice <= bidPrice {
		return nil, nil, nil, false
	}

	return foldShape(message, bidPrice, askPrice)
}

/*
foldShape projects one message's visible orders onto the folded bilateral and
signed whole-book shape coordinates for a known uncrossed touch. It is shared
by projectShape and projectShapeWithCache; ok is false on an empty side.
*/
func foldShape(message kraken.Level3Data, bidPrice float64, askPrice float64) ([]distribution.WeightedPoint, []distribution.WeightedPoint, []distribution.WeightedPoint, bool) {
	spread := askPrice - bidPrice
	midpoint := (bidPrice + askPrice) / 2

	bidFolded := make([]distribution.WeightedPoint, 0, len(message.Bids))
	askFolded := make([]distribution.WeightedPoint, 0, len(message.Asks))
	whole := make([]distribution.WeightedPoint, 0, len(message.Bids)+len(message.Asks))

	for _, order := range message.Bids {
		if !order.Resting() {
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
		if !order.Resting() {
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
