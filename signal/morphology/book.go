/*
Package morphology measures the shape of an order book as a geometric object:
where displayed notional sits along the price axis, how symmetric the two
sides' shapes are, how concentrated each side is, and how much the whole shape
moved since the last observation.
*/
package morphology

import (
	"fmt"
	"sort"
	"sync"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/distribution"
	"github.com/theapemachine/symm/nomagique/equation"
	types "github.com/theapemachine/symm/nomagique/types"
)

type symbolState struct {
	previousSec  float64
	previousNsec float64
	standardizer equation.Standardizer
	count        int
	hasTime      bool
}

/*
Book is the book-shape market entity. It reads the shared book per step and
projects one dimensionless shape Measurement. It retains the prior whole-book
shape per symbol — a single overwritten shape each, the same bounded-resident-
state contract as the shared book — so structural change is measured causally.
*/
type Book struct {
	mu         sync.Mutex
	previous   map[string][]distribution.WeightedPoint
	lastBid    map[string]float64
	lastAsk    map[string]float64
	lastBidRaw map[string][]distribution.WeightedPoint
	lastAskRaw map[string][]distribution.WeightedPoint
	states     map[string]*symbolState
}

func NewBook() *Book {
	return &Book{
		previous:   make(map[string][]distribution.WeightedPoint),
		lastBid:    make(map[string]float64),
		lastAsk:    make(map[string]float64),
		lastBidRaw: make(map[string][]distribution.WeightedPoint),
		lastAskRaw: make(map[string][]distribution.WeightedPoint),
		states:     make(map[string]*symbolState),
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

	id := fmt.Sprintf("morphology:%s:%d", message.Symbol, message.Timestamp.UnixNano())
	measurement := data.NewMeasurement[float64](id, message.Symbol, "morphology", message.Timestamp, message.Timestamp)
	measurement.Metadata = make(map[string]float64)

	putMetric(measurement, "book_shape_distance", shapeDistance, data.UnitDimensionless)
	putMetric(measurement, "book_shape_ks", shapeKS, data.UnitDimensionless)
	putMetric(measurement, "concentration:bid", concentrationBid, data.UnitDimensionless)
	putMetric(measurement, "concentration:ask", concentrationAsk, data.UnitDimensionless)
	putMetric(measurement, "entropy:bid", entropyBid, data.UnitNat)
	putMetric(measurement, "entropy:ask", entropyAsk, data.UnitNat)

	// Only a step that actually had a prior shape carries a structural change,
	// and only then is there anything for the estimator to measure. Without it
	// the shape facts still project; the measurement simply reports no SNR.
	if !changed {
		measurement.Finalize()

		return measurement
	}

	morphology.mu.Lock()
	state, found := morphology.states[message.Symbol]

	if !found {
		state = &symbolState{}
		morphology.states[message.Symbol] = state
	}

	msgSec := float64(message.Timestamp.Unix())
	msgNsec := float64(message.Timestamp.Nanosecond())

	if state.hasTime {
		if msgSec < state.previousSec || (msgSec == state.previousSec && msgNsec < state.previousNsec) {
			morphology.mu.Unlock()

			return nil
		}
	}

	state.previousSec = msgSec
	state.previousNsec = msgNsec
	state.hasTime = true
	state.count++

	putMetric(measurement, "morphology_change", morphologyChange, data.UnitDimensionless)

	// Causal pre-observation baseline & z-score:
	if state.count > 1 {
		baseline := state.standardizer.Mean()
		dispersion := state.standardizer.Dispersion()
		variance := state.standardizer.Variance()

		putMetric(measurement, "morphology_change_baseline", baseline, data.UnitDimensionless)

		if dispersion > 0 {
			zScore := (morphologyChange - baseline) / dispersion
			putMetric(measurement, "morphology_change_zscore", zScore, data.UnitDimensionless)
			measurement.Metadata[data.MetadataDivergence] = morphologyChange - baseline
			measurement.Metadata[data.MetadataNoiseVariance] = variance
		}
	}

	// Update standardizer with current observation
	state.standardizer.Step(types.Scalar(morphologyChange))
	morphology.mu.Unlock()

	measurement.Metadata[data.MetadataSupport] = float64(state.count)
	measurement.Finalize()

	return measurement
}

func putMetric(measurement *data.Measurement[float64], name string, value float64, unit data.Unit) {
	measurement.PutMetric(data.NewMetric(
		name, value, nil, nil, unit, data.TimescaleInstantaneous,
	))
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
