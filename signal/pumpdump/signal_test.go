package pumpdump

import (
	"context"
	"math"
	"slices"
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

type staticBookSource struct {
	book *spotbook.Book
}

func (source staticBookSource) Book(_ string, read func(*spotbook.Book)) {
	read(source.book)
}

func mgrBid() spotbook.BookDirection { return spotbook.Bid }
func mgrAsk() spotbook.BookDirection { return spotbook.Ask }

func logRatio(bid float64, ask float64) float64 {
	return math.Log(bid) - math.Log(ask)
}

func seededBook() *spotbook.Book {
	book := spotbook.New()
	book.Update(&spotbook.UpdateOptions{
		Direction: mgrBid(), ID: "bid", Price: decimal.NewFromInt64(99),
		Quantity: decimal.NewFromInt64(40), Timestamp: time.Unix(1, 0).UTC(),
		Silent: true,
	})
	book.Update(&spotbook.UpdateOptions{
		Direction: mgrAsk(), ID: "ask", Price: decimal.NewFromInt64(101),
		Quantity: decimal.NewFromInt64(10), Timestamp: time.Unix(1, 0).UTC(),
		Silent: true,
	})

	return book
}

func TestMeasure(t *testing.T) {
	Convey("Given a managed book and one executed trade", t, func() {
		signal := NewSignal(context.Background(), staticBookSource{book: seededBook()})
		market := types.NewSymbol("BTC/USD", nil)
		at := time.Unix(1_700_002_300, 0).UTC()
		market.AppendLevel3(kraken.Level3Data{
			Symbol: "BTC/USD", Type: "update", Timestamp: at,
		}, types.Level3Receivers)
		market.AppendTrade(pumpdumpTrade(1, "buy", 100, at.Add(time.Second)), types.TradeReceivers)

		measurements := slices.Collect(signal.Measure(market))

		Convey("It should always yield the ladder geometry with tape confirmation", func() {
			So(measurements, ShouldHaveLength, 1)
			measurement := measurements[0]
			So(measurement.Source, ShouldEqual, types.SourcePumpDump)
			So(measurement.Symbol, ShouldEqual, "BTC/USD")
			So(measurement.Sample(types.MetricLadderBidDepth, types.SideNone).Raw, ShouldEqual, 40.0)
			So(measurement.Sample(types.MetricLadderAskDepth, types.SideNone).Raw, ShouldEqual, 10.0)
			So(measurement.Sample(types.MetricLadderImbalance, types.SideNone).Raw,
				ShouldEqual, logRatio(40, 10))
			So(measurement.Sample(types.MetricBestPrice, types.SideBuy).Raw, ShouldEqual, 99.0)
			So(measurement.Sample(types.MetricBestPrice, types.SideSell).Raw, ShouldEqual, 101.0)
			So(measurement.Sample(types.MetricTradePrice, types.SideNone).Raw, ShouldEqual, 100.0)
			So(measurement.Maturity, ShouldBeGreaterThan, 0.0)
		})
	})

	Convey("Given a quoteless trade answered by the book's own touch", t, func() {
		signal := NewSignal(context.Background(), staticBookSource{book: seededBook()})
		market := types.NewSymbol("BTC/USD", nil)
		at := time.Unix(1_700_002_400, 0).UTC()
		market.AppendLevel3(kraken.Level3Data{
			Symbol: "BTC/USD", Type: "update", Timestamp: at,
		}, types.Level3Receivers)
		market.AppendTrade(pumpdumpTrade(7, "buy", 100, at.Add(time.Second)), types.TradeReceivers)

		measurements := slices.Collect(signal.Measure(market))

		Convey("It should never drop an executed print for lack of a ticker", func() {
			So(measurements, ShouldHaveLength, 1)
			So(measurements[0].Sample(types.MetricTradePrice, types.SideNone).Raw,
				ShouldEqual, 100.0)
		})
	})

	Convey("Given no book, no level3, and one trade without a quote", t, func() {
		signal := NewSignal(context.Background(), nil)
		market := types.NewSymbol("BTC/USD", nil)
		market.AppendTrade(pumpdumpTrade(2, "sell", 100, time.Unix(1_700_002_500, 0).UTC()), types.TradeReceivers)

		measurements := slices.Collect(signal.Measure(market))

		Convey("It should still always produce one measurement", func() {
			So(measurements, ShouldHaveLength, 1)
			So(measurements[0].Source, ShouldEqual, types.SourcePumpDump)
			So(signal.Name(), ShouldEqual, string(types.SourcePumpDump))
			So(signal.Type(), ShouldEqual, types.SourcePumpDump)
		})
	})

	Convey("Given a coiling ask side across passes", t, func() {
		base := time.Unix(1_700_002_600, 0).UTC()
		book := seededBook()
		signal := NewSignal(context.Background(), staticBookSource{book: book})
		market := types.NewSymbol("BTC/USD", nil)

		feed := func(at time.Time) []*types.Measurement {
			market.AppendLevel3(kraken.Level3Data{
				Symbol: "BTC/USD", Type: "update", Timestamp: at,
			}, types.Level3Receivers)

			return slices.Collect(signal.Measure(market))
		}

		first := feed(base)
		So(first, ShouldHaveLength, 1)
		So(first[0].Sample(types.MetricLadderAskDepletion, types.SideNone).Raw,
			ShouldEqual, 0.0)

		gap := time.Duration(system.Cfg.PumpDump.Halflife * 10 * float64(time.Second))

		// The resting ask is more than decimated a long event-time gap later.
		book.Update(&spotbook.UpdateOptions{
			Direction: mgrAsk(), ID: "ask", Price: decimal.NewFromInt64(101),
			Quantity:  decimal.NewFromInt64(2),
			Timestamp: base.Add(gap),
			Silent:    true,
		})

		coiled := feed(base.Add(gap))

		Convey("It should score the honest ask depletion against its own baseline", func() {
			So(coiled, ShouldHaveLength, 1)
			So(coiled[0].Sample(types.MetricLadderAskDepletion, types.SideNone).Raw,
				ShouldBeGreaterThan, 0.0)
		})
	})
}

func pumpdumpTrade(
	id int64,
	side string,
	price float64,
	at time.Time,
) kraken.TradeData {
	return kraken.TradeData{
		Symbol: "BTC/USD", Side: side, Price: *decimal.NewFromFloat64(price),
		Qty: 20, TradeID: id, Timestamp: at,
	}
}
