package toxicity

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	spotbook "github.com/theapemachine/api-go/v2/pkg/book"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestMeasure(t *testing.T) {
	Convey("Given unchanged touches for independent toxicity symbols", t, func() {
		books := &toxicityBookSource{books: make(map[string]*spotbook.Book)}
		signal := &Signal{ctx: context.Background(), books: books}
		base := time.Unix(1_700_004_100, 0).UTC()
		thesis := types.NewThesis(t.Context(), nil)
		thesis.AppendTicker(kraken.TickerData{Symbol: "BTC/USD", Timestamp: base})
		thesis.AppendTicker(kraken.TickerData{Symbol: "ALT/USD", Timestamp: base})
		books.books["BTC/USD"] = toxicityBook(100, 101, 10, 10, base)
		books.books["ALT/USD"] = toxicityBook(50, 51, 20, 20, base)

		Reset(func() {
			signal.Close()
		})

		Convey("It completes each symbol before returning the combined measurements", func() {
			measurements := signal.Measure(thesis)
			So(measurements, ShouldHaveLength, 2)
			thesis.AppendMeasurements(types.SourceToxicity, measurements, true)
			So(signal.Measure(thesis), ShouldBeEmpty)
		})
	})

	Convey("Given a pre-touch, multiple executions, and a later post-touch", t, func() {
		books := &toxicityBookSource{books: make(map[string]*spotbook.Book)}
		signal := &Signal{ctx: context.Background(), books: books}
		base := time.Unix(1_700_004_200, 0).UTC()
		thesis := types.NewThesis(t.Context(), nil)
		thesis.AppendTicker(kraken.TickerData{Symbol: "BTC/USD", Timestamp: base})
		books.books["BTC/USD"] = toxicityBook(100, 101, 10, 10, base)
		provisional := signal.Measure(thesis)
		thesis.AppendMeasurements(types.SourceToxicity, provisional, false)
		So(provisional, ShouldHaveLength, 1)
		So(provisional[0].At, ShouldResemble, base)
		So(*provisional[0].Sample(types.MetricTouchQuantity, types.SideBuy).Normalized,
			ShouldEqual, 0.5)
		So(provisional[0].Sample(types.MetricSNR, types.SideNone).Raw,
			ShouldEqual, 0.0)

		firstTrade := toxicityTrade(91, "sell", 100, 2, base.Add(time.Second))
		secondTrade := toxicityTrade(92, "buy", 101, 3, base.Add(2*time.Second))
		thesis.AppendTrade(firstTrade)
		thesis.AppendTrade(secondTrade)

		Convey("It keeps both trades pending until a strict post-trade touch arrives", func() {
			So(signal.Measure(thesis), ShouldBeEmpty)
			So(thesis.MarketTrades(types.SourceToxicity), ShouldHaveLength, 2)

			postAt := base.Add(3 * time.Second)
			books.books["BTC/USD"] = toxicityBook(99, 101, 7, 6, postAt)
			measurements := signal.Measure(thesis)
			So(measurements, ShouldHaveLength, 1)
			measurement := measurements[0]
			So(measurement.ObservedFrom, ShouldResemble, base)
			So(measurement.At, ShouldResemble, postAt)
			So(measurement.Horizon, ShouldEqual, 3*time.Second)
			So(measurement.Sample(types.MetricTradeVolume, types.SideNone).Raw,
				ShouldEqual, 5.0)
			So(measurement.Sample(types.MetricFillVolume, types.SideBuy).Raw,
				ShouldEqual, 2.0)
			So(measurement.Sample(types.MetricFillVolume, types.SideSell).Raw,
				ShouldEqual, 3.0)
			So(measurement.Sample(types.MetricRetreatingQuantity, types.SideBuy).Raw,
				ShouldEqual, 8.0)
			So(measurement.Sample(types.MetricCancelledQuantity, types.SideSell).Raw,
				ShouldEqual, 1.0)
			So(*measurement.Sample(types.MetricTradeVolume, types.SideNone).Normalized,
				ShouldAlmostEqual, 0.2, 1e-12)
			So(*measurement.Sample(types.MetricFillVolume, types.SideBuy).Normalized,
				ShouldAlmostEqual, 0.2, 1e-12)
			So(*measurement.Sample(types.MetricCancelledQuantity, types.SideSell).Normalized,
				ShouldAlmostEqual, 0.1, 1e-12)
			So(*measurement.Sample(types.MetricBestPrice, types.SideSell).Normalized,
				ShouldEqual, 0.0)
		})
	})

	Convey("Given only a pre-trade touch", t, func() {
		books := &toxicityBookSource{books: make(map[string]*spotbook.Book)}
		signal := &Signal{ctx: context.Background(), books: books}
		base := time.Unix(1_700_004_300, 0).UTC()
		thesis := types.NewThesis(t.Context(), nil)
		thesis.AppendTicker(kraken.TickerData{Symbol: "BTC/USD", Timestamp: base})
		books.books["BTC/USD"] = toxicityBook(100, 101, 10, 10, base)
		provisional := signal.Measure(thesis)
		thesis.AppendMeasurements(types.SourceToxicity, provisional, false)
		trade := toxicityTrade(93, "buy", 101, 1, base.Add(time.Second))
		thesis.AppendTrade(trade)

		Convey("It cannot claim validity or consume the unresolved trade", func() {
			So(signal.Measure(thesis), ShouldBeEmpty)
			trades := thesis.MarketTrades(types.SourceToxicity)
			So(trades, ShouldHaveLength, 1)
			So(trades[0].TradeID, ShouldEqual, trade.TradeID)
			So(trades[0].Timestamp, ShouldResemble, trade.Timestamp)
			So(thesis.Stamped("BTC/USD", types.SourceToxicity), ShouldBeFalse)
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

func BenchmarkToxicityMeasurement(b *testing.B) {
	from := time.Unix(1_700_004_400, 0).UTC()
	previous := touchSnapshot{
		asOf: from,
		bid:  touchObservation{price: 100, quantity: 10},
		ask:  touchObservation{price: 101, quantity: 10},
	}
	current := touchSnapshot{
		asOf: from.Add(time.Second),
		bid:  touchObservation{price: 99, quantity: 7},
		ask:  touchObservation{price: 101, quantity: 6},
	}
	trades := []kraken.TradeData{
		toxicityTrade(1, "sell", 100, 2, from.Add(time.Second)),
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = toxicityMeasurement("BTC/USD", previous, current, trades)
	}
}
