package toxicity

import (
	"math"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/types"
)

const (
	toxicityMetricTradeVolume           = "trade_volume"
	toxicityMetricFillVolumeBuy         = "fill_volume:buy"
	toxicityMetricFillVolumeSell        = "fill_volume:sell"
	toxicityMetricBestPriceBuy          = "best_price:buy"
	toxicityMetricBestPriceSell         = "best_price:sell"
	toxicityMetricTouchQuantityBuy      = "touch_quantity:buy"
	toxicityMetricTouchQuantitySell     = "touch_quantity:sell"
	toxicityMetricCancelledQuantityBuy  = "cancelled_quantity:buy"
	toxicityMetricCancelledQuantitySell = "cancelled_quantity:sell"
	toxicityMetricRetreatQuantityBuy    = "retreating_quantity:buy"
	toxicityMetricRetreatQuantitySell   = "retreating_quantity:sell"
)

/*
emitSymbolMeasurements builds the complete toxicity metric set for one symbol
and records the next touch snapshot for the following observation. Idle tape,
fill, cancel, and retreat evidence is published as zero rather than omitted so
consumers always see a stable metric identity.
*/
func (signal *Signal) emitSymbolMeasurements(
	symbol string,
	row *symbolEvidence,
) *types.Measurement {
	evidenceCount := row.tradeCount + row.bookCount
	maturity := float64(evidenceCount) / float64(evidenceCount+1)
	measurement := &types.Measurement{
		Source:   types.SourceToxicity,
		Symbol:   symbol,
		At:       row.latestAt,
		Maturity: maturity,
		Validity: types.ObservationValidity(evidenceCount),
		Scale: types.ScaleReference{
			Kind:    types.ScaleObservationWindow,
			From:    row.latestAt,
			Through: row.latestAt,
		},
		Metrics: make(map[string]types.MetricSample, 11),
	}

	addTapeMetrics(measurement, row)
	bookObserved := false

	peeked := signal.level3.PeekBook(symbol, func(symbolBook *book.Book) {
		if symbolBook.Name != symbol {
			return
		}

		bid, ask := symbolBook.BestBid(), symbolBook.BestAsk()

		if bid == nil || ask == nil {
			return
		}

		bidQuantity := bid.Quantity.Float64()
		askQuantity := ask.Quantity.Float64()
		bookObserved = true

		signal.pendingTouch[symbol] = touchSnapshot{
			bidPrice:    *bid.Price,
			askPrice:    *ask.Price,
			bidQuantity: bidQuantity,
			askQuantity: askQuantity,
			observedAt:  row.latestAt,
		}

		addTouchMetrics(measurement, bid, ask, bidQuantity, askQuantity)
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

	if !peeked || !bookObserved {
		addZeroTouchMetrics(measurement)
		putHonestyMetrics(
			measurement,
			0, 0,
			0, 0,
			0, 0,
			0, 0,
		)
	}

	return measurement
}

/*
addZeroTouchMetrics emits zero-valued touch evidence when Level3 is unavailable.
*/
func addZeroTouchMetrics(measurement *types.Measurement) {
	measurement.Metrics[toxicityMetricBestPriceBuy] = types.MetricSample{
		Raw:  0,
		Unit: types.UnitQuoteCurrency,
	}
	measurement.Metrics[toxicityMetricBestPriceSell] = types.MetricSample{
		Raw:  0,
		Unit: types.UnitQuoteCurrency,
	}
	measurement.Metrics[toxicityMetricTouchQuantityBuy] = types.MetricSample{
		Raw:        0,
		Normalized: types.NormalizeRatio(0, 0),
		Unit:       types.UnitBaseCurrency,
	}
	measurement.Metrics[toxicityMetricTouchQuantitySell] = types.MetricSample{
		Raw:        0,
		Normalized: types.NormalizeRatio(0, 0),
		Unit:       types.UnitBaseCurrency,
	}
}

/*
addTapeMetrics emits trade volume and both fill sides every observation.
*/
func addTapeMetrics(measurement *types.Measurement, row *symbolEvidence) {
	measurement.Metrics[toxicityMetricTradeVolume] = types.MetricSample{
		Raw:  row.volume,
		Unit: types.UnitBaseCurrency,
	}
	measurement.Metrics[toxicityMetricFillVolumeBuy] = types.MetricSample{
		Raw: row.fillBid,
		// Honesty is executed base size versus observed trade size; notional
		// fill value is retained as Raw for quote-space reporting only.
		Normalized: types.NormalizeRatio(
			row.bidExecuted, math.Max(row.volume, row.bidExecuted),
		),
		Unit: types.UnitQuoteCurrency,
	}
	measurement.Metrics[toxicityMetricFillVolumeSell] = types.MetricSample{
		Raw: row.fillAsk,
		Normalized: types.NormalizeRatio(
			row.askExecuted, math.Max(row.volume, row.askExecuted),
		),
		Unit: types.UnitQuoteCurrency,
	}
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
	measurement.Metrics[toxicityMetricBestPriceBuy] = types.MetricSample{
		Raw:  bid.Price.Float64(),
		Unit: types.UnitQuoteCurrency,
	}
	measurement.Metrics[toxicityMetricBestPriceSell] = types.MetricSample{
		Raw:  ask.Price.Float64(),
		Unit: types.UnitQuoteCurrency,
	}
	measurement.Metrics[toxicityMetricTouchQuantityBuy] = types.MetricSample{
		Raw: bidQuantity,
		Normalized: types.NormalizeRatio(
			bidQuantity, bidQuantity+askQuantity,
		),
		Unit: types.UnitBaseCurrency,
	}
	measurement.Metrics[toxicityMetricTouchQuantitySell] = types.MetricSample{
		Raw: askQuantity,
		Normalized: types.NormalizeRatio(
			askQuantity, bidQuantity+askQuantity,
		),
		Unit: types.UnitBaseCurrency,
	}
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
		putHonestyMetrics(measurement, 0, 0, 0, 0, 0, 0, 0, 0)

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

	retreatingSide, retreatingQuantity, retreatBase := dominantRetreat(
		bidCancelled, askCancelled, prior.bidQuantity, prior.askQuantity,
	)
	bidRetreatEmit := 0.0
	askRetreatEmit := 0.0
	bidRetreatBase := prior.bidQuantity
	askRetreatBase := prior.askQuantity

	switch retreatingSide {
	case types.SideBuy:
		bidRetreatEmit = retreatingQuantity
		bidRetreatBase = retreatBase
	case types.SideSell:
		askRetreatEmit = retreatingQuantity
		askRetreatBase = retreatBase
	}

	putHonestyMetrics(
		measurement,
		bidCancelEmit, askCancelEmit,
		bidRetreatEmit, askRetreatEmit,
		prior.bidQuantity, prior.askQuantity,
		bidRetreatBase, askRetreatBase,
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
	cancelPriorBid float64,
	cancelPriorAsk float64,
	retreatPriorBid float64,
	retreatPriorAsk float64,
) {
	measurement.Metrics[toxicityMetricCancelledQuantityBuy] = types.MetricSample{
		Raw:        bidCancelled,
		Normalized: types.NormalizeRatio(bidCancelled, cancelPriorBid),
		Unit:       types.UnitBaseCurrency,
	}
	measurement.Metrics[toxicityMetricCancelledQuantitySell] = types.MetricSample{
		Raw:        askCancelled,
		Normalized: types.NormalizeRatio(askCancelled, cancelPriorAsk),
		Unit:       types.UnitBaseCurrency,
	}
	measurement.Metrics[toxicityMetricRetreatQuantityBuy] = types.MetricSample{
		Raw:        bidRetreat,
		Normalized: types.NormalizeRatio(bidRetreat, retreatPriorBid),
		Unit:       types.UnitBaseCurrency,
	}
	measurement.Metrics[toxicityMetricRetreatQuantitySell] = types.MetricSample{
		Raw:        askRetreat,
		Normalized: types.NormalizeRatio(askRetreat, retreatPriorAsk),
		Unit:       types.UnitBaseCurrency,
	}
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
