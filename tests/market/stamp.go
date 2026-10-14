package market

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Quote replaces symbol's book with a level3 snapshot holding one order on each
side, the way a venue snapshot seeds the book.
*/
func Quote(book *broker.Book, symbol string, at time.Time, bid, bidQty, ask, askQty float64) error {
	entry := kraken.Level3Data{
		Symbol: symbol,
		Bids:   []kraken.Level3Order{},
		Asks:   []kraken.Level3Order{},
	}

	if bidQty > 0 {
		entry.Bids = append(entry.Bids, kraken.Level3Order{
			OrderID: symbol + "-bid", LimitPrice: decimal.NewFromFloat64(bid),
			OrderQty: decimal.NewFromFloat64(bidQty), Timestamp: at, Event: "add",
		})
	}

	if askQty > 0 {
		entry.Asks = append(entry.Asks, kraken.Level3Order{
			OrderID: symbol + "-ask", LimitPrice: decimal.NewFromFloat64(ask),
			OrderQty: decimal.NewFromFloat64(askQty), Timestamp: at, Event: "add",
		})
	}

	return book.Update(&kraken.Level3{
		Channel: "level3",
		Type:    "snapshot",
		Data:    []kraken.Level3Data{entry},
	})
}

/*
Stamp builds one spot:trade frame the way ingress builds it: the exact price,
the quantity, the aggressor side, and the snapshot book holds at the trade's
sequence number, taken through broker.Book.Stamp. A book that is unseeded,
stale, one-sided, or crossed leaves the trade unstamped, as ingress leaves it.
*/
func Stamp(book *broker.Book, symbol, side string, at time.Time, price, qty float64, extra ...*data.Metric) *data.Measurement {
	_, snapshot := book.Stamp(symbol)
	measurement := data.NewMeasurement(
		1, symbol, "spot:trade", at, at,
		&data.StringEntry{Key: "type", Value: "trade"},
		&data.StringEntry{Key: "side", Value: side},
	)

	priceUnit, priceTime := data.GetDimension("spot:trade", "price")
	qtyUnit, qtyTime := data.GetDimension("spot:trade", "qty")
	metrics := []*data.Metric{
		data.NewExactMetric("price", decimal.NewFromFloat64(price), priceUnit, priceTime),
		data.NewMetric("qty", qty, qtyUnit, qtyTime),
	}

	if snapshot != nil {
		metrics = append(metrics, snapshot.Metrics()...)
	}

	metrics = append(metrics, extra...)

	return measurement.Write(metrics...)
}

/*
Unreadable builds a spot:trade frame whose stamped book cannot be decoded: a
best bid level without the depth window every stamp carries. Valid ingress
never produces it.
*/
func Unreadable(symbol, side string, at time.Time, price, qty float64) *data.Measurement {
	measurement := data.NewMeasurement(
		1, symbol, "spot:trade", at, at,
		&data.StringEntry{Key: "type", Value: "trade"},
		&data.StringEntry{Key: "side", Value: side},
	)

	priceUnit, priceTime := data.GetDimension("spot:trade", "price")
	qtyUnit, qtyTime := data.GetDimension("spot:trade", "qty")

	return measurement.Write(
		data.NewExactMetric("price", decimal.NewFromFloat64(price), priceUnit, priceTime),
		data.NewMetric("qty", qty, qtyUnit, qtyTime),
		data.NewExactMetric("bid_price_0", decimal.NewFromFloat64(price), priceUnit, priceTime),
	)
}
