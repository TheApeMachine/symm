/*
Package morphology measures the shape of an order book as a geometric object:
where displayed notional sits along the price axis, how symmetric the two
sides' shapes are, how concentrated each side is, and how much the whole shape
moved since the last observation.
*/
package morphology

import (
	"context"
	"fmt"
	"sort"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/distribution"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/types"
)

type symbolState struct {
	previousSec  float64
	previousNsec float64
	moments      statistic.Moments
	residual     core.Primitive
	count        int
	hasTime      bool
}

/*
Signal is the book-morphology measuring instrument. It composes its market
entity in its constructor and exposes the canonical signal structure:
Constructor, Name, Error, Step, Close.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error

	book *Book
}

func NewSignal(ctx context.Context) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		book:   NewBook(),
	}
}

func (signal *Signal) Name() string { return "morphology" }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Step(envelope *types.Envelope) *types.Envelope {
	if signal.err != nil {
		errnie.Error(signal.Close())
		return nil
	}

	if envelope.TypeID != types.EnvelopeLevel3 {
		return envelope
	}

	envelope.Morphology = signal.book.Step(envelope.Level3Data)

	return envelope
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return signal.book.Close()
}

/*
Book is the book-shape market entity. It reads the shared book per step and
projects one dimensionless shape Measurement. It retains the prior whole-book
shape per symbol — a single overwritten shape each, the same bounded-resident-
state contract as the shared book — so structural change is measured causally.
*/
type Book struct {
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

	shapeDistanceEval := transport.NewEvaluate(distribution.NewWasserstein1Pairs())
	var shapeDistance float64

	for out := range shapeDistanceEval.Next(
		transport.NewValues(distribution.PairsInput{Left: bidFolded, Right: askFolded}).Next(nil),
	) {
		shapeDistance = *(*float64)(out)
	}

	if err := shapeDistanceEval.Error(); err != nil {
		return nil
	}

	shapeKSEval := transport.NewEvaluate(distribution.NewKolmogorovSmirnovPairs())
	var shapeKS float64

	for out := range shapeKSEval.Next(
		transport.NewValues(distribution.PairsInput{Left: bidFolded, Right: askFolded}).Next(nil),
	) {
		shapeKS = *(*float64)(out)
	}

	if err := shapeKSEval.Error(); err != nil {
		return nil
	}

	concentrationBidEval := transport.NewEvaluate(distribution.NewConcentrationPoints())
	var concentrationBid float64

	for out := range concentrationBidEval.Next(
		transport.NewValues(distribution.PointsInput{Points: bidFolded}).Next(nil),
	) {
		concentrationBid = *(*float64)(out)
	}

	if err := concentrationBidEval.Error(); err != nil {
		return nil
	}

	concentrationAskEval := transport.NewEvaluate(distribution.NewConcentrationPoints())
	var concentrationAsk float64

	for out := range concentrationAskEval.Next(
		transport.NewValues(distribution.PointsInput{Points: askFolded}).Next(nil),
	) {
		concentrationAsk = *(*float64)(out)
	}

	if err := concentrationAskEval.Error(); err != nil {
		return nil
	}

	entropyBidEval := transport.NewEvaluate(distribution.NewEntropyPoints())
	var entropyBid float64

	for out := range entropyBidEval.Next(
		transport.NewValues(distribution.PointsInput{Points: bidFolded}).Next(nil),
	) {
		entropyBid = *(*float64)(out)
	}

	if err := entropyBidEval.Error(); err != nil {
		return nil
	}

	entropyAskEval := transport.NewEvaluate(distribution.NewEntropyPoints())
	var entropyAsk float64

	for out := range entropyAskEval.Next(
		transport.NewValues(distribution.PointsInput{Points: askFolded}).Next(nil),
	) {
		entropyAsk = *(*float64)(out)
	}

	if err := entropyAskEval.Error(); err != nil {
		return nil
	}

	morphologyChange, changed, err := morphology.recordChange(message.Symbol, whole)

	if err != nil {
		return nil
	}

	id := fmt.Sprintf("morphology:%s:%d", message.Symbol, message.Timestamp.UnixNano())
	measurement := data.NewMeasurement[float64]("morphology", nil)
	measurement.Label, measurement.At, measurement.From = message.Symbol, message.Timestamp, message.Timestamp
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

	state, found := morphology.states[message.Symbol]

	if !found {
		state = &symbolState{residual: statistic.NewCausalResidual()}
		morphology.states[message.Symbol] = state
	}

	msgSec := float64(message.Timestamp.Unix())
	msgNsec := float64(message.Timestamp.Nanosecond())

	if state.hasTime {
		if msgSec < state.previousSec || (msgSec == state.previousSec && msgNsec < state.previousNsec) {
			return nil
		}
	}

	state.previousSec = msgSec
	state.previousNsec = msgNsec
	state.hasTime = true
	state.count++

	putMetric(measurement, "morphology_change", morphologyChange, data.UnitDimensionless)

	reading := state.moments.Update(morphologyChange)
	residualEval := transport.NewEvaluate(state.residual)
	var result statistic.CausalResidualResult

	for out := range residualEval.Next(
		transport.NewOne(unsafe.Pointer(&reading)).Next(nil),
	) {
		result = *(*statistic.CausalResidualResult)(out)
	}

	if err := residualEval.Error(); err != nil {
		measurement.Err = err

		return measurement
	}

	if result.HasPrior {
		putMetric(measurement, "morphology_change_baseline", result.Baseline, data.UnitDimensionless)

		if result.PriorVariance > 0 {
			putMetric(measurement, "morphology_change_zscore", result.ZScore, data.UnitDimensionless)
			measurement.Metadata[data.MetadataDivergence] = result.Residual
			measurement.Metadata[data.MetadataNoiseVariance] = result.PriorVariance
		}
	}

	measurement.Metadata[data.MetadataSupport] = float64(state.count)
	measurement.Finalize()

	return measurement
}

func putMetric(measurement *data.Measurement[float64], name string, value float64, unit data.Unit) {
	measurement.Metrics[name] = data.Metric[float64]{
		Label: name, Raw: value, Unit: unit, Timescale: data.TimescaleInstantaneous,
	}
}

/*
recordChange stores the current whole-book shape and returns how far it moved
from the previous shape of the same symbol, on their merged position streams.
The first observation of a symbol has no prior and reports no change. Ownership
of the current slice transfers into the resident map, so no extra clone is made.
*/
func (morphology *Book) recordChange(symbol string, current []distribution.WeightedPoint) (float64, bool, error) {
	previous, hadPrevious := morphology.previous[symbol]
	morphology.previous[symbol] = current

	if !hadPrevious {
		return 0, false, nil
	}

	distanceEval := transport.NewEvaluate(distribution.NewWasserstein1Pairs())
	var distance float64

	for out := range distanceEval.Next(
		transport.NewValues(distribution.PairsInput{Left: previous, Right: current}).Next(nil),
	) {
		distance = *(*float64)(out)
	}

	if err := distanceEval.Error(); err != nil {
		return 0, false, err
	}

	return distance, true, nil
}

/*
projectShapeWithCache projects one Level3Data message as projectShape does,
but borrows each side's last observed raw orders when the message is one-sided
(Kraken sends Level-3 as 1-sided incremental updates), re-folding the borrowed
side against the current touch so the shape reflects the resting book assumed
unchanged on the absent side. See projectShape.
*/
func (morphology *Book) projectShapeWithCache(message kraken.Level3Data) ([]distribution.WeightedPoint, []distribution.WeightedPoint, []distribution.WeightedPoint, bool) {
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
