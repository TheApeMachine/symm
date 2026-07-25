package toxicity

import (
	"math"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/types"
)

/*
emitSymbolMeasurements appends the complete toxicity metric set for one symbol
and records the next touch snapshot for the following observation. Idle tape,
fill, cancel, and retreat evidence is published as zero rather than omitted so
consumers always see a stable metric identity.
*/
func (signal *Signal) emitSymbolMeasurements(
	symbol string,
	row *symbolEvidence,
	out *[]*types.Measurement,
	nextTouch map[string]touchSnapshot,
) error {
	evidenceCount := row.tradeCount + row.bookCount
	maturity := float64(evidenceCount) / float64(evidenceCount+1)
	obsCtx := newObservationContext(row.latestAt, evidenceCount)
	measurement := symbolMeasurement(symbol, row.latestAt, maturity, obsCtx)

	addTapeMetrics(measurement, row)

	signal.level3.PeekBook(symbol, func(symbolBook *book.Book) {
		if symbolBook.Name != symbol {
			return
		}

		bid, ask := symbolBook.BestBid(), symbolBook.BestAsk()

		if bid == nil || ask == nil {
			return
		}

		bidQuantity := bid.Quantity.Float64()
		askQuantity := ask.Quantity.Float64()

		nextTouch[symbol] = touchSnapshot{
			bidPrice:    *bid.Price,
			askPrice:    *ask.Price,
			bidQuantity: bidQuantity,
			askQuantity: askQuantity,
			observedAt:  row.latestAt,
		}

		addTouchMetrics(
			measurement, bid, ask, bidQuantity, askQuantity,
		)
		addHonestyMetrics(
			measurement,
			row,
			signal.priorTouch[symbol],
			bid.Price,
			ask.Price,
			bidQuantity,
			askQuantity,
		)
	})

	*out = append(*out, measurement)

	return nil
}

/*
symbolMeasurement allocates one source×symbol row for the full toxicity set.
*/
func symbolMeasurement(
	symbol string,
	at time.Time,
	maturity float64,
	obsCtx observationContext,
) *types.Measurement {
	return &types.Measurement{
		Source:   types.SourceToxicity,
		Symbol:   symbol,
		At:       at,
		Maturity: maturity,
		Validity: obsCtx.validity,
		Scale:    obsCtx.scale,
		Metrics:  map[string]types.MetricSample{},
	}
}

/*
addTapeMetrics emits trade volume and both fill sides every observation.
*/
func addTapeMetrics(measurement *types.Measurement, row *symbolEvidence) {
	measurement.PutMetric(
		types.MetricTradeVolume, types.SideNone,
		types.MetricSample{
			Raw:  row.volume,
			Unit: types.UnitBaseCurrency,
		},
	)
	measurement.PutMetric(
		types.MetricFillVolume, types.SideBuy,
		types.MetricSample{
			Raw: row.fillBid,
			// Honesty is executed base size versus observed trade size; notional
			// fill value is retained as Raw for quote-space reporting only.
			Normalized: types.NormalizeRatio(
				row.bidExecuted, math.Max(row.volume, row.bidExecuted),
			),
			Unit: types.UnitQuoteCurrency,
		},
	)
	measurement.PutMetric(
		types.MetricFillVolume, types.SideSell,
		types.MetricSample{
			Raw: row.fillAsk,
			Normalized: types.NormalizeRatio(
				row.askExecuted, math.Max(row.volume, row.askExecuted),
			),
			Unit: types.UnitQuoteCurrency,
		},
	)
}

/*
addTouchMetrics emits best-price and touch-quantity evidence for both sides.
*/
func addTouchMetrics(
	measurement *types.Measurement,
	bid *book.Level,
	ask *book.Level,
	bidQuantity float64,
	askQuantity float64,
) {
	measurement.PutMetric(
		types.MetricBestPrice, types.SideBuy,
		types.MetricSample{
			Raw:  bid.Price.Float64(),
			Unit: types.UnitQuoteCurrency,
		},
	)
	measurement.PutMetric(
		types.MetricBestPrice, types.SideSell,
		types.MetricSample{
			Raw:  ask.Price.Float64(),
			Unit: types.UnitQuoteCurrency,
		},
	)
	measurement.PutMetric(
		types.MetricTouchQuantity, types.SideBuy,
		types.MetricSample{
			Raw: bidQuantity,
			Normalized: types.NormalizeRatio(
				bidQuantity, bidQuantity+askQuantity,
			),
			Unit: types.UnitBaseCurrency,
		},
	)
	measurement.PutMetric(
		types.MetricTouchQuantity, types.SideSell,
		types.MetricSample{
			Raw: askQuantity,
			Normalized: types.NormalizeRatio(
				askQuantity, bidQuantity+askQuantity,
			),
			Unit: types.UnitBaseCurrency,
		},
	)
}

/*
addHonestyMetrics compares current and prior touch quantities against executions
so cancellations are not mistaken for fills.
*/
func addHonestyMetrics(
	measurement *types.Measurement,
	row *symbolEvidence,
	prior touchSnapshot,
	bidPrice *decimal.Decimal,
	askPrice *decimal.Decimal,
	bidQuantity float64,
	askQuantity float64,
) {
	if prior.bidQuantity <= 0 && prior.askQuantity <= 0 {
		putHonestyMetrics(measurement, 0, 0, 0, 0, 0, 0)

		return
	}

	bidRetreated := bidPrice.Cmp(&prior.bidPrice) < 0
	askRetreated := askPrice.Cmp(&prior.askPrice) > 0
	bidCancelled := cancelledQuantity(
		prior.bidQuantity, bidQuantity, row.bidExecuted, bidRetreated,
	)
	askCancelled := cancelledQuantity(
		prior.askQuantity, askQuantity, row.askExecuted, askRetreated,
	)

	bidCancelEmit := 0.0
	askCancelEmit := 0.0

	if bidCancelled > 0 && !bidRetreated {
		bidCancelEmit = bidCancelled
	}

	if askCancelled > 0 && !askRetreated {
		askCancelEmit = askCancelled
	}

	retreatingSide, retreatingQuantity, _ := dominantRetreat(
		bidCancelled, askCancelled, prior.bidQuantity, prior.askQuantity,
	)
	bidRetreatEmit := 0.0
	askRetreatEmit := 0.0

	switch retreatingSide {
	case types.SideBuy:
		bidRetreatEmit = retreatingQuantity
	case types.SideSell:
		askRetreatEmit = retreatingQuantity
	}

	putHonestyMetrics(
		measurement,
		bidCancelEmit, askCancelEmit,
		bidRetreatEmit, askRetreatEmit,
		prior.bidQuantity, prior.askQuantity,
	)
}

/*
putHonestyMetrics builds cancel and retreat samples for both sides.
*/
func putHonestyMetrics(
	measurement *types.Measurement,
	bidCancelled float64,
	askCancelled float64,
	bidRetreat float64,
	askRetreat float64,
	priorBid float64,
	priorAsk float64,
) {
	measurement.PutMetric(
		types.MetricCancelledQuantity, types.SideBuy,
		types.MetricSample{
			Raw:        bidCancelled,
			Normalized: types.NormalizeRatio(bidCancelled, priorBid),
			Unit:       types.UnitBaseCurrency,
		},
	)
	measurement.PutMetric(
		types.MetricCancelledQuantity, types.SideSell,
		types.MetricSample{
			Raw:        askCancelled,
			Normalized: types.NormalizeRatio(askCancelled, priorAsk),
			Unit:       types.UnitBaseCurrency,
		},
	)
	measurement.PutMetric(
		types.MetricRetreatingQuantity, types.SideBuy,
		types.MetricSample{
			Raw:        bidRetreat,
			Normalized: types.NormalizeRatio(bidRetreat, priorBid),
			Unit:       types.UnitBaseCurrency,
		},
	)
	measurement.PutMetric(
		types.MetricRetreatingQuantity, types.SideSell,
		types.MetricSample{
			Raw:        askRetreat,
			Normalized: types.NormalizeRatio(askRetreat, priorAsk),
			Unit:       types.UnitBaseCurrency,
		},
	)
}

/*
cancelledQuantity removes known executions from a touch-size decline so only
unexplained withdrawal is classified as cancellation.
*/
func cancelledQuantity(
	priorQuantity float64,
	currentQuantity float64,
	executedQuantity float64,
	retreated bool,
) float64 {
	if retreated {
		return math.Max(0, priorQuantity-executedQuantity)
	}

	removal := priorQuantity - currentQuantity

	if removal <= 0 {
		return 0
	}

	return math.Max(0, removal-executedQuantity)
}

/*
dominantRetreat selects the side with the larger withdrawn fraction so unequal
touch sizes remain comparable without collapsing side identity.
*/
func dominantRetreat(
	bidCancelled float64,
	askCancelled float64,
	priorBid float64,
	priorAsk float64,
) (types.MeasurementSide, float64, float64) {
	bidPressure := retreatPressure(bidCancelled, priorBid)
	askPressure := retreatPressure(askCancelled, priorAsk)

	if bidPressure <= 0 && askPressure <= 0 {
		return types.SideNone, 0, 0
	}

	if askPressure > bidPressure {
		return types.SideSell, askCancelled, priorAsk
	}

	return types.SideBuy, bidCancelled, priorBid
}

/*
retreatPressure expresses withdrawn quantity relative to its own prior touch.
*/
func retreatPressure(cancelled float64, priorTouch float64) float64 {
	if cancelled <= 0 || priorTouch <= 0 {
		return 0
	}

	return cancelled / priorTouch
}
