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
		askQuantity < 0 || !finite(bidPrice) || !finite(askPrice) ||
		!finite(bidQuantity) || !finite(askQuantity) {
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
			bidQuantity < 0 || askQuantity < 0 || !finite(bidPrice) ||
			!finite(askPrice) || !finite(bidQuantity) || !finite(askQuantity) {
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
	tradeVolumeNormalized := normalizedTouchRatio(
		tradeVolume,
		previous.bid.quantity+previous.ask.quantity,
	)
	bidFillNormalized := normalizedTouchRatio(bidFill, previous.bid.quantity)
	askFillNormalized := normalizedTouchRatio(askFill, previous.ask.quantity)
	bidPriceNormalized := normalizedTouchPrice(current.bid.price, previous.bid.price)
	askPriceNormalized := normalizedTouchPrice(current.ask.price, previous.ask.price)
	bidQuantityNormalized := normalizedTouchRatio(current.bid.quantity, previous.bid.quantity)
	askQuantityNormalized := normalizedTouchRatio(current.ask.quantity, previous.ask.quantity)
	bidRetreatNormalized := normalizedTouchRatio(bidRetreat, previous.bid.quantity)
	askRetreatNormalized := normalizedTouchRatio(askRetreat, previous.ask.quantity)
	bidCancelledNormalized := normalizedTouchRatio(bidCancelled, previous.bid.quantity)
	askCancelledNormalized := normalizedTouchRatio(askCancelled, previous.ask.quantity)

	return &types.Measurement{
		ID:           uuid.NewString(),
		Source:       types.SourceToxicity,
		Symbol:       symbol,
		At:           current.asOf,
		ObservedFrom: previous.asOf,
		Horizon:      current.asOf.Sub(previous.asOf),
		Metrics: map[string]types.MetricSample{
			types.MetricKey(types.MetricTradeVolume, types.SideNone): {
				Raw:        tradeVolume,
				Normalized: tradeVolumeNormalized,
				Unit:       types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricFillVolume, types.SideBuy): {
				Raw:        bidFill,
				Normalized: bidFillNormalized,
				Unit:       types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricFillVolume, types.SideSell): {
				Raw:        askFill,
				Normalized: askFillNormalized,
				Unit:       types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricBestPrice, types.SideBuy): {
				Raw:        current.bid.price,
				Normalized: bidPriceNormalized,
				Unit:       types.UnitQuoteCurrency,
			},
			types.MetricKey(types.MetricBestPrice, types.SideSell): {
				Raw:        current.ask.price,
				Normalized: askPriceNormalized,
				Unit:       types.UnitQuoteCurrency,
			},
			types.MetricKey(types.MetricTouchQuantity, types.SideBuy): {
				Raw:        current.bid.quantity,
				Normalized: bidQuantityNormalized,
				Unit:       types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricTouchQuantity, types.SideSell): {
				Raw:        current.ask.quantity,
				Normalized: askQuantityNormalized,
				Unit:       types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricRetreatingQuantity, types.SideBuy): {
				Raw:        bidRetreat,
				Normalized: bidRetreatNormalized,
				Unit:       types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricRetreatingQuantity, types.SideSell): {
				Raw:        askRetreat,
				Normalized: askRetreatNormalized,
				Unit:       types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricCancelledQuantity, types.SideBuy): {
				Raw:        bidCancelled,
				Normalized: bidCancelledNormalized,
				Unit:       types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricCancelledQuantity, types.SideSell): {
				Raw:        askCancelled,
				Normalized: askCancelledNormalized,
				Unit:       types.UnitBaseCurrency,
			},
		},
	}
}

/*
normalizedTouchRatio expresses executed, remaining, retreating, and cancelled
base quantity against the resting touch quantity that could actually change.
*/
func normalizedTouchRatio(raw, previousQuantity float64) *float64 {
	value := raw / previousQuantity

	return &value
}

/*
normalizedTouchPrice is the side-specific log return between the two observed
touches. An unchanged, valid price therefore produces a genuine zero.
*/
func normalizedTouchPrice(currentPrice, previousPrice float64) *float64 {
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

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
