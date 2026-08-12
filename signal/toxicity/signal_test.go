package toxicity

import (
	"context"
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestMeasure(t *testing.T) {
	Convey("Given toxicity book observations on one symbol", t, func() {
		books := &toxicityBookSource{books: make(map[string]*spotbook.Book)}
		signal := NewSignal(context.Background(), books, nil)
		market := types.NewSymbol("BTC/USD", nil)
		base := time.Unix(1_700_004_100, 0).UTC()
		books.books["BTC/USD"] = toxicityBook(100, 101, 10, 10, base)

		measurements := signal.Measure(market)

		Convey("It should emit book-quality input metrics from nomagique", func() {
			So(measurements, ShouldHaveLength, 1)
			measurement := measurements[0]
			So(measurement.Source, ShouldEqual, types.SourceToxicity)
			So(measurement.Symbol, ShouldEqual, "BTC/USD")
			So(measurement.Sample(types.MetricTouchQuantity, types.SideBuy).Raw, ShouldEqual, 10.0)
			So(measurement.Sample(types.MetricTouchQuantity, types.SideSell).Raw, ShouldEqual, 10.0)
		})
	})

	Convey("Given trades after retained book state", t, func() {
		books := &toxicityBookSource{books: make(map[string]*spotbook.Book)}
		signal := NewSignal(context.Background(), books, nil)
		market := types.NewSymbol("BTC/USD", nil)
		base := time.Unix(1_700_004_200, 0).UTC()
		books.books["BTC/USD"] = toxicityBook(100, 101, 10, 10, base)
		So(signal.Measure(market), ShouldHaveLength, 1)
		market.AppendTrade(toxicityTrade(91, "sell", 100, 2, base.Add(time.Second)))

		measurements := signal.Measure(market)

		Convey("It should append trade and book measurements", func() {
			So(measurements, ShouldHaveLength, 2)
			So(measurements[0].Sample(types.MetricTradeVolume, types.SideNone).Raw, ShouldEqual, 2.0)
		})
	})
}

type toxicityBookSource struct {
	books map[string]*spotbook.Book
}

func (source *toxicityBookSource) Book(symbol string, read func(*spotbook.Book)) {
	read(source.books[symbol])
}

func toxicityTrade(
	id int64,
	side string,
	price float64,
	quantity float64,
	at time.Time,
) kraken.TradeData {
	return kraken.TradeData{
		Symbol: "BTC/USD", Side: side, Price: *decimal.NewFromFloat64(price),
		Qty: quantity, TradeID: id, Timestamp: at,
	}
}

func toxicityBook(
	bidPrice float64,
	askPrice float64,
	bidQuantity float64,
	askQuantity float64,
	at time.Time,
) *spotbook.Book {
	managed := spotbook.New()
	managed.Name = "BTC/USD"
	managed.NoBookCrossing = false
	managed.Update(&spotbook.UpdateOptions{
		Direction: spotbook.Bid, Price: decimal.NewFromFloat64(bidPrice),
		Quantity: decimal.NewFromFloat64(bidQuantity), Timestamp: at,
	})
	managed.Update(&spotbook.UpdateOptions{
		Direction: spotbook.Ask, Price: decimal.NewFromFloat64(askPrice),
		Quantity: decimal.NewFromFloat64(askQuantity), Timestamp: at,
	})

	return managed
}
