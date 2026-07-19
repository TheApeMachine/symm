package toxicity

import (
	"math"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/types"
)

/*
emitSymbolMeasurements appends tape and touch measurements for one symbol and
records the next touch snapshot for the following observation.
*/
func (signal *Signal) emitSymbolMeasurements(
	symbol string,
	row *symbolEvidence,
	out *[]*types.Measurement,
	nextTouch map[string]touchSnapshot,
) {
	maturity := float64(row.tradeCount) / float64(row.tradeCount+1)
	obsCtx := newObservationContext(row.latestAt, row.tradeCount)

	*out = append(*out, tapeMeasurements(symbol, row, maturity, obsCtx)...)

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
			bidQuantity: bidQuantity,
			askQuantity: askQuantity,
			observedAt:  row.latestAt,
		}

		*out = append(*out, touchMeasurements(
			symbol, row, bid, ask, bidQuantity, askQuantity, maturity, obsCtx,
		)...)

		honesty := signal.touchHonesty(
			symbol,
			row,
			signal.priorTouch[symbol],
			bidQuantity,
			askQuantity,
			maturity,
		)

		*out = append(*out, honesty...)
	})
}

/*
tapeMeasurements emits trade volume and optional bid or ask fill evidence.
*/
func tapeMeasurements(
	symbol string,
	row *symbolEvidence,
	maturity float64,
	obsCtx observationContext,
) []*types.Measurement {
	measurements := []*types.Measurement{
		{
			Source:   types.SourceToxicity,
			Stream:   types.Toxicity,
			Metric:   types.MetricTradeVolume,
			Subject:  types.SubjectLevel3Tape,
			Symbol:   symbol,
			At:       row.latestAt,
			Unit:     types.UnitBaseCurrency,
			Raw:      row.volume,
			Maturity: maturity,
			Validity: obsCtx.validity,
			Scale:    obsCtx.scale,
		},
	}

	if row.fillBid > 0 {
		measurements = append(measurements, &types.Measurement{
			Source:   types.SourceToxicity,
			Stream:   types.Toxicity,
			Metric:   types.MetricFillVolume,
			Subject:  types.SubjectLevel3Tape,
			Symbol:   symbol,
			Side:     types.SideBuy,
			At:       row.latestAt,
			Unit:     types.UnitQuoteCurrency,
			Raw:      row.fillBid,
			Maturity: maturity,
			Validity: obsCtx.validity,
			Scale:    obsCtx.scale,
		})
	}

	if row.fillAsk > 0 {
		measurements = append(measurements, &types.Measurement{
			Source:   types.SourceToxicity,
			Stream:   types.Toxicity,
			Metric:   types.MetricFillVolume,
			Subject:  types.SubjectLevel3Tape,
			Symbol:   symbol,
			Side:     types.SideSell,
			At:       row.latestAt,
			Unit:     types.UnitQuoteCurrency,
			Raw:      row.fillAsk,
			Maturity: maturity,
			Validity: obsCtx.validity,
			Scale:    obsCtx.scale,
		})
	}

	return measurements
}

/*
touchMeasurements emits best-price and touch-quantity evidence for both sides.
*/
func touchMeasurements(
	symbol string,
	row *symbolEvidence,
	bid *book.Level,
	ask *book.Level,
	bidQuantity float64,
	askQuantity float64,
	maturity float64,
	obsCtx observationContext,
) []*types.Measurement {
	return []*types.Measurement{
		{
			Source:   types.SourceToxicity,
			Stream:   types.Toxicity,
			Metric:   types.MetricBestPrice,
			Subject:  types.SubjectLevel3Touch,
			Symbol:   symbol,
			Side:     types.SideBuy,
			At:       row.latestAt,
			Unit:     types.UnitQuoteCurrency,
			Raw:      bid.Price.Float64(),
			Maturity: maturity,
			Validity: obsCtx.validity,
			Scale:    obsCtx.scale,
		},
		{
			Source:   types.SourceToxicity,
			Stream:   types.Toxicity,
			Metric:   types.MetricBestPrice,
			Subject:  types.SubjectLevel3Touch,
			Symbol:   symbol,
			Side:     types.SideSell,
			At:       row.latestAt,
			Unit:     types.UnitQuoteCurrency,
			Raw:      ask.Price.Float64(),
			Maturity: maturity,
			Validity: obsCtx.validity,
			Scale:    obsCtx.scale,
		},
		{
			Source:  types.SourceToxicity,
			Stream:  types.Toxicity,
			Metric:  types.MetricTouchQuantity,
			Subject: types.SubjectLevel3Touch,
			Symbol:  symbol,
			Side:    types.SideBuy,
			At:      row.latestAt,
			Unit:    types.UnitBaseCurrency,
			Raw:     bidQuantity,
			Normalized: types.NormalizeRatio(
				bidQuantity, bidQuantity+askQuantity,
			),
			Maturity: maturity,
			Validity: obsCtx.validity,
			Scale:    obsCtx.scale,
		},
		{
			Source:  types.SourceToxicity,
			Stream:  types.Toxicity,
			Metric:  types.MetricTouchQuantity,
			Subject: types.SubjectLevel3Touch,
			Symbol:  symbol,
			Side:    types.SideSell,
			At:      row.latestAt,
			Unit:    types.UnitBaseCurrency,
			Raw:     askQuantity,
			Normalized: types.NormalizeRatio(
				askQuantity, bidQuantity+askQuantity,
			),
			Maturity: maturity,
			Validity: obsCtx.validity,
			Scale:    obsCtx.scale,
		},
	}
}

/*
touchHonesty compares current and prior touch quantities against executions so
cancellations are not mistaken for fills.
*/
func (signal *Signal) touchHonesty(
	symbol string,
	row *symbolEvidence,
	prior touchSnapshot,
	bidQuantity float64,
	askQuantity float64,
	maturity float64,
) []*types.Measurement {
	if prior.bidQuantity <= 0 && prior.askQuantity <= 0 {
		return nil
	}

	bidCancelled := cancelledQuantity(prior.bidQuantity, bidQuantity, row.bidExecuted)
	askCancelled := cancelledQuantity(prior.askQuantity, askQuantity, row.askExecuted)
	measurements := make([]*types.Measurement, 0, 4)
	obsCtx := newObservationContext(row.latestAt, row.tradeCount)

	if bidCancelled > 0 {
		measurements = append(measurements, &types.Measurement{
			Source:     types.SourceToxicity,
			Stream:     types.Toxicity,
			Metric:     types.MetricCancelledQuantity,
			Subject:    types.SubjectLevel3Touch,
			Symbol:     symbol,
			Side:       types.SideBuy,
			At:         row.latestAt,
			Unit:       types.UnitBaseCurrency,
			Raw:        bidCancelled,
			Normalized: types.NormalizeRatio(bidCancelled, prior.bidQuantity),
			Maturity:   maturity,
			Validity:   obsCtx.validity,
			Scale:      obsCtx.scale,
		})
	}

	if askCancelled > 0 {
		measurements = append(measurements, &types.Measurement{
			Source:     types.SourceToxicity,
			Stream:     types.Toxicity,
			Metric:     types.MetricCancelledQuantity,
			Subject:    types.SubjectLevel3Touch,
			Symbol:     symbol,
			Side:       types.SideSell,
			At:         row.latestAt,
			Unit:       types.UnitBaseCurrency,
			Raw:        askCancelled,
			Normalized: types.NormalizeRatio(askCancelled, prior.askQuantity),
			Maturity:   maturity,
			Validity:   obsCtx.validity,
			Scale:      obsCtx.scale,
		})
	}

	retreatingSide, retreatingQuantity, retreatBaseline := dominantRetreat(
		bidCancelled, askCancelled, prior.bidQuantity, prior.askQuantity,
	)

	if retreatingQuantity <= 0 {
		return measurements
	}

	measurements = append(measurements, &types.Measurement{
		Source:     types.SourceToxicity,
		Stream:     types.Toxicity,
		Metric:     types.MetricRetreatingQuantity,
		Subject:    types.SubjectLevel3Touch,
		Symbol:     symbol,
		Side:       retreatingSide,
		At:         row.latestAt,
		Unit:       types.UnitBaseCurrency,
		Raw:        retreatingQuantity,
		Normalized: types.NormalizeRatio(retreatingQuantity, retreatBaseline),
		Maturity:   maturity,
		Validity:   obsCtx.validity,
		Scale:      obsCtx.scale,
	})

	return measurements
}

func cancelledQuantity(
	priorQuantity float64,
	currentQuantity float64,
	executedQuantity float64,
) float64 {
	removal := priorQuantity - currentQuantity

	if removal <= 0 {
		return 0
	}

	return math.Max(0, removal-executedQuantity)
}

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

func retreatPressure(cancelled float64, priorTouch float64) float64 {
	if cancelled <= 0 || priorTouch <= 0 {
		return 0
	}

	return cancelled / priorTouch
}
