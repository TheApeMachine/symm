package strategy

import (
	"math"
	"math/big"
	"strconv"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
Detector turns a quote tape into resolved episodes. A departure is only an
anchor once the next quote stays outside the calm dispersion, and it becomes
a record only when a later measured shift marks the episode's end.
*/
type Detector struct {
	price  *broker.Price
	series map[string]*series
	err    error
}

type quote struct {
	symbol string
	seq    int64
	bid    *decimal.Decimal
	ask    *decimal.Decimal
	mid    float64
}

type series struct {
	symbol         string
	lastSeq        int64
	baseline       statistic.Moments
	regimeStart    int64
	calmCount      int
	longestImpulse int
	calmHigh       float64
	calmLow        float64
	calmHighTick   int64
	calmLowTick    int64
	calmHighBid    *decimal.Decimal
	calmLowAsk     *decimal.Decimal
	calmEntryAsk   *decimal.Decimal
	candidate      bool
	candidateSeq   int64
	side           int
	priorMean      float64
	priorDisp      float64
	held           quote
	open           bool
	anchor         int64
	precursor      int64
	calmDisp       float64
	entryAsk       *decimal.Decimal
	highMid        float64
	lowMid         float64
	highBid        *decimal.Decimal
	lowAsk         *decimal.Decimal
	highTick       int64
	lowTick        int64
	impulse        statistic.Moments
	recent         statistic.Moments
	openCount      int
}

func NewDetector(price *broker.Price) *Detector {
	return &Detector{
		price:  price,
		series: make(map[string]*series),
	}
}

func (detector *Detector) Error() error {
	if detector == nil {
		return nil
	}

	return detector.err
}

/*
Observe accepts one measurement. Quotes without both sides, non-positive
prices, a crossed book, or a repeated sequence produce no record. A missing
fee leaves the episode unresolved until the fee surface has it; a negative
fee is an error and that episode is not emitted.
*/
func (detector *Detector) Observe(measurement *data.Measurement[float64]) (*tables.ExcursionRecord, error) {
	if detector == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"episode: detector is required",
			nil,
		))
	}

	seen, ok := quoteFrom(measurement)

	if !ok {
		return nil, nil
	}

	path := detector.series[seen.symbol]

	if path == nil {
		path = &series{symbol: seen.symbol}
		detector.series[seen.symbol] = path
	}

	if seen.seq <= path.lastSeq {
		return nil, nil
	}

	path.lastSeq = seen.seq

	if path.open {
		return path.advance(detector, seen)
	}

	if path.candidate {
		return path.confirm(detector, seen)
	}

	return path.calm(detector, seen)
}

func (path *series) calm(detector *Detector, seen quote) (*tables.ExcursionRecord, error) {
	prior := path.baseline

	if prior.Count > 1 {
		dispersion := math.Sqrt(prior.M2 / (prior.Count - 1))
		shift := seen.mid - prior.Mean

		if dispersion > 0 && (shift > dispersion || shift < -dispersion) {
			path.candidate = true
			path.candidateSeq = seen.seq
			path.priorMean = prior.Mean
			path.priorDisp = dispersion
			path.held = seen
			path.side = 0

			if shift > 0 {
				path.side = 1
			}

			if shift < 0 {
				path.side = -1
			}

			return nil, nil
		}
	}

	path.baseline.Update(seen.mid)
	path.absorbCalm(seen)

	return path.maybeFlat(detector, seen)
}

func (path *series) confirm(detector *Detector, seen quote) (*tables.ExcursionRecord, error) {
	shift := seen.mid - path.priorMean
	outside := path.side > 0 && shift > path.priorDisp

	if path.side < 0 && shift < -path.priorDisp {
		outside = true
	}

	if !outside {
		path.baseline.Update(path.held.mid)
		path.baseline.Update(seen.mid)
		path.absorbCalm(path.held)
		path.absorbCalm(seen)
		path.candidate = false
		path.held = quote{}

		return path.maybeFlat(detector, seen)
	}

	path.open = true
	path.candidate = false
	path.anchor = path.candidateSeq
	path.precursor = path.regimeStart
	path.calmDisp = path.priorDisp
	path.entryAsk = path.held.ask
	path.impulse = statistic.Moments{}
	path.recent = statistic.Moments{}
	path.highBid = nil
	path.lowAsk = nil
	path.include(path.held)
	path.include(seen)
	path.openCount = 2
	path.recent.Update(seen.mid)

	return nil, nil
}

func (path *series) advance(detector *Detector, seen quote) (*tables.ExcursionRecord, error) {
	path.openCount++
	extended := seen.mid > path.highMid || seen.mid < path.lowMid
	path.include(seen)

	if extended {
		path.recent = statistic.Moments{}
		path.recent.Update(seen.mid)

		return nil, nil
	}

	path.recent.Update(seen.mid)
	priorCount := path.impulse.Count - path.recent.Count

	if path.recent.Count <= 1 || priorCount <= 1 || path.impulse.Count <= 1 {
		return nil, nil
	}

	bound := adaptive.MeanShift{
		Variance:     path.impulse.M2 / (path.impulse.Count - 1),
		Observations: path.impulse.Count,
		RecentCount:  path.recent.Count,
		PriorCount:   priorCount,
	}.Bound()

	if bound <= 0 {
		return nil, nil
	}

	extreme := path.highMid

	if path.priorMean-path.lowMid > path.highMid-path.priorMean {
		extreme = path.lowMid
	}

	gap := extreme - path.recent.Mean

	if gap < 0 {
		gap = -gap
	}

	if gap <= bound {
		return nil, nil
	}

	return path.finish(detector, seen)
}

func (path *series) finish(detector *Detector, seen quote) (*tables.ExcursionRecord, error) {
	completed := path.openCount
	fee, err := detector.fee(path.symbol)

	if err != nil {
		detector.err = err
		path.restart(seen, completed)

		return nil, err
	}

	if fee == nil {
		return nil, nil
	}

	record, err := path.classify(seen, fee)

	if err != nil {
		detector.err = err
		path.restart(seen, completed)

		return nil, err
	}

	path.restart(seen, completed)

	return record, nil
}

func (path *series) classify(seen quote, fee *decimal.Decimal) (*tables.ExcursionRecord, error) {
	if !positiveFinite(path.priorMean) || path.entryAsk == nil || seen.bid == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"episode: resolved quote has no executable price",
			nil,
		))
	}

	one := decimal.NewFromInt64(1)
	cost := path.entryAsk.Mul(one.Add(fee))
	proceeds := seen.bid.Mul(one.Sub(fee))
	clears := proceeds.Cmp(cost) > 0
	upDisp := path.highMid - path.priorMean
	downDisp := path.priorMean - path.lowMid
	upMove := upDisp > path.calmDisp
	downMove := downDisp > path.calmDisp
	direction := ""
	friction := false

	if upMove && downMove && !clears {
		direction = "chop"
	}

	if direction == "" && upMove && upDisp >= downDisp && clears {
		direction = "up"
		friction = true
	}

	if direction == "" && upMove && upDisp >= downDisp {
		direction = "up"
	}

	if direction == "" && downMove && downDisp > upDisp {
		direction = "down"
	}

	if direction == "" {
		return nil, nil
	}

	return path.record(seen, fee, cost, proceeds, direction, friction, false)
}

func (path *series) maybeFlat(detector *Detector, seen quote) (*tables.ExcursionRecord, error) {
	if path.longestImpulse <= 0 || path.calmCount <= path.longestImpulse {
		return nil, nil
	}

	fee, err := detector.fee(path.symbol)

	if err != nil {
		detector.err = err
		path.regimeStart = seen.seq
		path.calmCount = 1

		return nil, err
	}

	if fee == nil {
		return nil, nil
	}

	if path.calmEntryAsk == nil || seen.bid == nil || !positiveFinite(path.baseline.Mean) {
		path.regimeStart = seen.seq
		path.calmCount = 1

		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"episode: flat quote has no executable price",
			nil,
		))
	}

	one := decimal.NewFromInt64(1)
	cost := path.calmEntryAsk.Mul(one.Add(fee))
	proceeds := seen.bid.Mul(one.Sub(fee))
	record, err := path.record(seen, fee, cost, proceeds, "flat", false, true)

	if err != nil {
		detector.err = err
		path.regimeStart = seen.seq
		path.calmCount = 1

		return nil, err
	}

	path.regimeStart = seen.seq
	path.calmCount = 1
	path.calmEntryAsk = seen.ask
	path.calmHigh = seen.mid
	path.calmLow = seen.mid
	path.calmHighBid = seen.bid
	path.calmLowAsk = seen.ask
	path.calmHighTick = seen.seq
	path.calmLowTick = seen.seq

	return record, nil
}

func (path *series) record(
	seen quote,
	fee *decimal.Decimal,
	cost *decimal.Decimal,
	proceeds *decimal.Decimal,
	direction string,
	friction bool,
	flat bool,
) (*tables.ExcursionRecord, error) {
	anchor := path.anchor
	precursor := path.precursor
	extremumTick := path.highTick
	extremumPrice := path.highBid
	extremeMid := path.highMid
	count := path.openCount

	if direction == "down" {
		extremumTick = path.lowTick
		extremumPrice = path.lowAsk
		extremeMid = path.lowMid
	}

	if flat {
		anchor = path.regimeStart
		precursor = path.regimeStart
		extremumTick = path.calmHighTick
		extremumPrice = path.calmHighBid
		extremeMid = path.calmHigh
		count = path.calmCount

		if path.baseline.Mean-path.calmLow > path.calmHigh-path.baseline.Mean {
			extremumTick = path.calmLowTick
			extremumPrice = path.calmLowAsk
			extremeMid = path.calmLow
		}
	}

	reference := path.priorMean

	if flat {
		reference = path.baseline.Mean
	}

	// ProfitFraction and GrossExcursion are stored as float64. Computing them
	// through Decimal.Div panics when the divisor's unscaled integer rounds to
	// zero under the numerator's scale (krakenfx BankersRound QuoRem), even if
	// Sign() is non-zero — common when NewFromFloat64 yields scale 0 for an
	// integer-valued mid. Ratios stay honest: refuse non-positive / non-finite
	// divisors rather than inventing a stand-in.
	if cost == nil || cost.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"episode: cost is not a positive executable price",
			nil,
		))
	}

	if !positiveFinite(reference) || !finiteFloat(extremeMid) {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"episode: gross reference is not a positive finite price",
			nil,
		))
	}

	if extremumPrice == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"episode: extremum price is missing",
			nil,
		))
	}

	profit := proceeds.Sub(cost)
	fraction, ok := decimalRatio(profit, cost)

	if !ok {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"episode: profit fraction is undefined",
			nil,
		))
	}

	gross := (extremeMid - reference) / reference

	if !finiteFloat(gross) || !finiteFloat(fraction) {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"episode: excursion ratio is non-finite",
			nil,
		))
	}

	return &tables.ExcursionRecord{
		ID:                 path.symbol + ":" + formatInt(anchor) + ":" + formatInt(seen.seq),
		Symbol:             path.symbol,
		Direction:          direction,
		ClearsFriction:     friction,
		PrecursorStartTick: precursor,
		AnchorTick:         anchor,
		ExtremumTick:       extremumTick,
		ExitTick:           seen.seq,
		PostEndTick:        seen.seq,
		EntryPrice:         cost.Float64(),
		ExtremumPrice:      extremumPrice.Float64(),
		ExitPrice:          proceeds.Float64(),
		PositionSize:       1,
		Fee:                fee.Float64(),
		Profit:             profit.Float64(),
		ProfitFraction:     fraction,
		GrossExcursion:     gross,
		ObservationCount:   int64(count),
		Status:             "resolved",
	}, nil
}

func (path *series) restart(seen quote, completed int) {
	longest := path.longestImpulse

	if completed > longest {
		longest = completed
	}

	symbol := path.symbol
	last := path.lastSeq
	*path = series{
		symbol:         symbol,
		lastSeq:        last,
		longestImpulse: longest,
		regimeStart:    seen.seq,
		calmCount:      1,
	}
	path.baseline.Update(seen.mid)
	path.noteCalm(seen)
}

func (path *series) absorbCalm(seen quote) {
	path.calmCount++

	if path.regimeStart == 0 {
		path.regimeStart = seen.seq
	}

	path.noteCalm(seen)
}

func (path *series) noteCalm(seen quote) {
	if path.calmEntryAsk == nil {
		path.calmEntryAsk = seen.ask
		path.calmHigh = seen.mid
		path.calmLow = seen.mid
		path.calmHighBid = seen.bid
		path.calmLowAsk = seen.ask
		path.calmHighTick = seen.seq
		path.calmLowTick = seen.seq

		return
	}

	if seen.mid >= path.calmHigh {
		path.calmHigh = seen.mid
		path.calmHighBid = seen.bid
		path.calmHighTick = seen.seq
	}

	if seen.mid <= path.calmLow {
		path.calmLow = seen.mid
		path.calmLowAsk = seen.ask
		path.calmLowTick = seen.seq
	}
}

func (path *series) include(seen quote) {
	path.impulse.Update(seen.mid)

	if path.highBid == nil || seen.mid >= path.highMid {
		path.highMid = seen.mid
		path.highBid = seen.bid
		path.highTick = seen.seq
	}

	if path.lowAsk == nil || seen.mid <= path.lowMid {
		path.lowMid = seen.mid
		path.lowAsk = seen.ask
		path.lowTick = seen.seq
	}
}

func (detector *Detector) fee(symbol string) (*decimal.Decimal, error) {
	if detector.price == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"episode: price is required to read the fee",
			nil,
		))
	}

	schedule := detector.price.FeeIfAvailable(symbol)

	if schedule == nil || schedule.Fee == nil {
		return nil, nil
	}

	if schedule.Fee.Sign() < 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"episode: fee is negative for "+symbol,
			nil,
		))
	}

	// TradeVolumeFee.Fee is a percentage (Kraken / paper). Classification uses a fraction.
	// Order matches broker.Price.WithFee: (1/100).Mul(fee). fee.Mul(1/100) truncates at scale 0.
	return decimal.NewFromInt64(1).Div(decimal.NewFromInt64(100)).Mul(schedule.Fee), nil
}

func quoteFrom(measurement *data.Measurement[float64]) (quote, bool) {
	if measurement == nil || measurement.SeqIdx <= 0 || measurement.Label == "" {
		return quote{}, false
	}

	bidMetric, bidOK := measurement.LookupMetric("bid")
	askMetric, askOK := measurement.LookupMetric("ask")

	if !bidOK || !askOK {
		return quote{}, false
	}

	bid := decimalPrice(bidMetric)
	ask := decimalPrice(askMetric)

	if bid == nil || ask == nil || bid.Sign() <= 0 || ask.Sign() <= 0 || ask.Cmp(bid) < 0 {
		return quote{}, false
	}

	mid := bid.Add(ask).Div(decimal.NewFromInt64(2)).Float64()

	return quote{
		symbol: measurement.Label,
		seq:    measurement.SeqIdx,
		bid:    bid,
		ask:    ask,
		mid:    mid,
	}, true
}

func decimalPrice(metric data.Metric[float64]) *decimal.Decimal {
	if metric.Exact != nil {
		return metric.Exact
	}

	return decimal.NewFromFloat64(metric.Raw)
}

func positiveFinite(value float64) bool {
	return value > 0 && finiteFloat(value)
}

func finiteFloat(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

/*
decimalRatio returns num/den as float64 without Decimal.Div. Div panics when
den.Sign() is non-zero but den rounds to a zero integer under num's scale.
*/
func decimalRatio(num *decimal.Decimal, den *decimal.Decimal) (float64, bool) {
	if num == nil || den == nil || den.Sign() == 0 {
		return 0, false
	}

	ratio, _ := new(big.Rat).Quo(num.Rat(), den.Rat()).Float64()

	if !finiteFloat(ratio) {
		return 0, false
	}

	return ratio, true
}

func formatInt(value int64) string {
	return strconv.FormatInt(value, 10)
}
