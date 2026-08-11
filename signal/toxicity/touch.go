package toxicity

import (
	"math"
	"time"

	"github.com/google/uuid"
	spotbook "github.com/theapemachine/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type touchObservation struct {
	price    float64
	quantity float64
}

type touchSnapshot struct {
	asOf time.Time
	bid  touchObservation
	ask  touchObservation
}

func observedTouch(managed *spotbook.Book) (touchSnapshot, bool) {
	if managed == nil {
		return touchSnapshot{}, false
	}

	bid := managed.BestBid()
	ask := managed.BestAsk()

	if bid == nil || ask == nil || bid.Price == nil || ask.Price == nil ||
		bid.Quantity == nil || ask.Quantity == nil {
		return touchSnapshot{}, false
	}

	bidPrice := bid.Price.Float64()
	askPrice := ask.Price.Float64()
	bidQuantity := bid.Quantity.Float64()
	askQuantity := ask.Quantity.Float64()
	asOf := bid.Timestamp

	if ask.Timestamp.After(asOf) {
		asOf = ask.Timestamp
	}

	if asOf.IsZero() || bidPrice <= 0 || askPrice <= bidPrice || bidQuantity < 0 ||
		askQuantity < 0 {
		return touchSnapshot{}, false
	}

	return touchSnapshot{
		asOf: asOf,
		bid:  touchObservation{price: bidPrice, quantity: bidQuantity},
		ask:  touchObservation{price: askPrice, quantity: askQuantity},
	}, true
}

func latestTouch(
	measurements []*types.Measurement,
	symbol string,
) (touchSnapshot, bool) {
	var latest touchSnapshot
	found := false

	for _, measurement := range measurements {
		if measurement == nil || measurement.Symbol != symbol ||
			measurement.Source != types.SourceToxicity ||
			measurement.At.Before(latest.asOf) {
			continue
		}

		bidPrice := measurement.Sample(types.MetricBestPrice, types.SideBuy).Raw
		askPrice := measurement.Sample(types.MetricBestPrice, types.SideSell).Raw
		bidQuantity := measurement.Sample(types.MetricTouchQuantity, types.SideBuy).Raw
		askQuantity := measurement.Sample(types.MetricTouchQuantity, types.SideSell).Raw

		if measurement.At.IsZero() || bidPrice <= 0 || askPrice <= bidPrice ||
			bidQuantity < 0 || askQuantity < 0 {
			continue
		}

		latest = touchSnapshot{
			asOf: measurement.At,
			bid: touchObservation{
				price:    bidPrice,
				quantity: bidQuantity,
			},
			ask: touchObservation{
				price:    askPrice,
				quantity: askQuantity,
			},
		}
		found = true
	}

	return latest, found
}

func toxicityMeasurement(
	symbol string,
	previous touchSnapshot,
	current touchSnapshot,
	trades []kraken.TradeData,
) *types.Measurement {
	if previous.asOf.Equal(current.asOf) {
		return &types.Measurement{
			ID:     uuid.NewString(),
			Source: types.SourceToxicity,
			Symbol: symbol,
			At:     current.asOf,
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricBestPrice, types.SideBuy): {
					Raw: current.bid.price, Unit: types.UnitQuoteCurrency,
				},
				types.MetricKey(types.MetricBestPrice, types.SideSell): {
					Raw: current.ask.price, Unit: types.UnitQuoteCurrency,
				},
				types.MetricKey(types.MetricTouchQuantity, types.SideBuy): {
					Raw: current.bid.quantity, Unit: types.UnitBaseCurrency,
				},
				types.MetricKey(types.MetricTouchQuantity, types.SideSell): {
					Raw: current.ask.quantity, Unit: types.UnitBaseCurrency,
				},
			},
		}
	}

	tradeVolume := 0.0
	bidFill := 0.0
	askFill := 0.0

	for _, trade := range trades {
		tradeVolume += trade.Qty

		if trade.Side == "sell" && trade.Price.Float64() == previous.bid.price {
			bidFill += trade.Qty
		}

		if trade.Side == "buy" && trade.Price.Float64() == previous.ask.price {
			askFill += trade.Qty
		}
	}

	bidFill = math.Min(bidFill, previous.bid.quantity)
	askFill = math.Min(askFill, previous.ask.quantity)
	bidRetreat, bidCancelled := touchChange(types.SideBuy, previous.bid, current.bid, bidFill)
	askRetreat, askCancelled := touchChange(types.SideSell, previous.ask, current.ask, askFill)

	measurement := &types.Measurement{
		ID:           uuid.NewString(),
		Source:       types.SourceToxicity,
		Symbol:       symbol,
		At:           current.asOf,
		ObservedFrom: previous.asOf,
		Horizon:      current.asOf.Sub(previous.asOf),
		Metrics: map[string]types.MetricSample{
			types.MetricKey(types.MetricTradeVolume, types.SideNone): {
				Raw: tradeVolume,
				Normalized: normalizedTouchRatio(
					tradeVolume,
					previous.bid.quantity+previous.ask.quantity,
					true,
				),
				Unit: types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricFillVolume, types.SideBuy): {
				Raw:        bidFill,
				Normalized: normalizedTouchRatio(bidFill, previous.bid.quantity, false),
				Unit:       types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricFillVolume, types.SideSell): {
				Raw:        askFill,
				Normalized: normalizedTouchRatio(askFill, previous.ask.quantity, false),
				Unit:       types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricBestPrice, types.SideBuy): {
				Raw:        current.bid.price,
				Normalized: normalizedTouchPrice(current.bid.price, previous.bid.price),
				Unit:       types.UnitQuoteCurrency,
			},
			types.MetricKey(types.MetricBestPrice, types.SideSell): {
				Raw:        current.ask.price,
				Normalized: normalizedTouchPrice(current.ask.price, previous.ask.price),
				Unit:       types.UnitQuoteCurrency,
			},
			types.MetricKey(types.MetricTouchQuantity, types.SideBuy): {
				Raw:        current.bid.quantity,
				Normalized: normalizedTouchRatio(current.bid.quantity, previous.bid.quantity, true),
				Unit:       types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricTouchQuantity, types.SideSell): {
				Raw:        current.ask.quantity,
				Normalized: normalizedTouchRatio(current.ask.quantity, previous.ask.quantity, true),
				Unit:       types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricRetreatingQuantity, types.SideBuy): {
				Raw:        bidRetreat,
				Normalized: normalizedTouchRatio(bidRetreat, previous.bid.quantity, false),
				Unit:       types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricRetreatingQuantity, types.SideSell): {
				Raw:        askRetreat,
				Normalized: normalizedTouchRatio(askRetreat, previous.ask.quantity, false),
				Unit:       types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricCancelledQuantity, types.SideBuy): {
				Raw:        bidCancelled,
				Normalized: normalizedTouchRatio(bidCancelled, previous.bid.quantity, false),
				Unit:       types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricCancelledQuantity, types.SideSell): {
				Raw:        askCancelled,
				Normalized: normalizedTouchRatio(askCancelled, previous.ask.quantity, false),
				Unit:       types.UnitBaseCurrency,
			},
		},
	}
	snr, snrReady := types.MeasurementSignalNoiseRatio(
		types.SourceToxicity,
		measurement.Metrics,
	)
	snrSample := types.MetricSample{
		Raw:  snr,
		Unit: types.UnitDimensionless,
	}

	if snrReady && current.asOf.After(previous.asOf) {
		snrSample.Normalized = &snr
	}

	measurement.PutMetric(types.MetricSNR, types.SideNone, snrSample)

	return measurement
}

/*
normalizedTouchRatio reports causal fill, retreat, and cancellation fractions.
For an unbounded current or traded quantity, competing selects its share against
the previous resting quantity instead of mislabelling an unbounded ratio.
*/
func normalizedTouchRatio(raw, previousQuantity float64, competing bool) *float64 {
	if raw < 0 || previousQuantity <= 0 {
		return nil
	}

	if !competing && raw > previousQuantity {
		return nil
	}

	value := raw / previousQuantity

	if competing {
		value = raw / (raw + previousQuantity)
	}

	return &value
}

/*
normalizedTouchPrice is the side-specific log return between the two observed
touches. An unchanged, valid price therefore produces a genuine zero.
*/
func normalizedTouchPrice(currentPrice, previousPrice float64) *float64 {
	if currentPrice <= 0 || previousPrice <= 0 {
		return nil
	}

	value := math.Log(currentPrice / previousPrice)

	return &value
}

func touchChange(
	side types.MeasurementSide,
	previous touchObservation,
	current touchObservation,
	executed float64,
) (float64, float64) {
	if previous.quantity <= 0 {
		return 0, 0
	}

	if side == types.SideBuy && current.price < previous.price {
		return math.Max(0, previous.quantity-executed), 0
	}

	if side == types.SideSell && current.price > previous.price {
		return math.Max(0, previous.quantity-executed), 0
	}

	if current.price != previous.price || current.quantity >= previous.quantity {
		return 0, 0
	}

	disappeared := previous.quantity - current.quantity

	return 0, math.Max(0, disappeared-executed)
}
