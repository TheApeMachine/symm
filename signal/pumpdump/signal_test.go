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
		})
		market.AppendTrade(pumpdumpTrade(1, "buy", 100, at.Add(time.Second)))

		err := signal.Measure(market)
		So(err, ShouldBeNil)
		measurements := slices.Collect(market.MarketMeasurements("category"))

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
		})
		market.AppendTrade(pumpdumpTrade(7, "buy", 100, at.Add(time.Second)))

		err := signal.Measure(market)
		So(err, ShouldBeNil)
		measurements := slices.Collect(market.MarketMeasurements("category"))

		Convey("It should never drop an executed print for lack of a ticker", func() {
			So(measurements, ShouldHaveLength, 1)
			So(measurements[0].Sample(types.MetricTradePrice, types.SideNone).Raw,
				ShouldEqual, 100.0)
		})
	})

	Convey("Given no book, no level3, and one trade without a quote", t, func() {
		signal := NewSignal(context.Background(), nil)
		market := types.NewSymbol("BTC/USD", nil)
		market.AppendTrade(pumpdumpTrade(2, "sell", 100, time.Unix(1_700_002_500, 0).UTC()))

		err := signal.Measure(market)
		So(err, ShouldBeNil)
		measurements := slices.Collect(market.MarketMeasurements("category"))

		Convey("It should still always produce one measurement", func() {
			So(measurements, ShouldHaveLength, 1)
			So(measurements[0].Source, ShouldEqual, types.SourcePumpDump)
			So(signal.Name(), ShouldEqual, string(types.SourcePumpDump))
			So(signal.Type(), ShouldEqual, types.SourcePumpDump)
		})
	})

	Convey("Given a trade without any observed ladder event time", t, func() {
		// The ladder clock is event time only. A pass with no level3 event time
		// carries no ladder observation, but the trade tape still yields.
		future := seededBook()
		future.Update(&spotbook.UpdateOptions{
			Direction: mgrBid(), ID: "bid", Price: decimal.NewFromFloat64(99),
			Quantity: decimal.NewFromFloat64(40), Timestamp: time.Now().UTC().Add(time.Hour),
			Silent: true,
		})
		future.Update(&spotbook.UpdateOptions{
			Direction: mgrAsk(), ID: "ask", Price: decimal.NewFromFloat64(101),
			Quantity: decimal.NewFromFloat64(10), Timestamp: time.Now().UTC().Add(time.Hour),
			Silent: true,
		})

		signal := NewSignal(context.Background(), staticBookSource{book: future})
		market := types.NewSymbol("BTC/USD", nil)
		market.AppendTrade(pumpdumpTrade(3, "buy", 100, time.Now().UTC()))

		err := signal.Measure(market)
		So(err, ShouldBeNil)
		measurements := slices.Collect(market.MarketMeasurements("category"))

		Convey("It should skip the ladder but still measure the tape", func() {
			So(measurements, ShouldHaveLength, 1)
			// No level3 event time was observed, so ladder geometry is absent.
			_, ladderBidDepth := measurements[0].Metrics[
				types.MetricKey(types.MetricLadderBidDepth, types.SideNone),
			]
			So(ladderBidDepth, ShouldBeFalse)
			So(measurements[0].Sample(types.MetricTradePrice, types.SideNone).Raw,
				ShouldEqual, 100.0)
		})
	})

	Convey("Given a coiling ask side across passes", t, func() {
		base := time.Unix(1_700_002_600, 0).UTC()
		book := seededBook()
		signal := NewSignal(context.Background(), staticBookSource{book: book})
		market := types.NewSymbol("BTC/USD", nil)

		feed := func(at time.Time) []types.Measurement {
			market.AppendLevel3(kraken.Level3Data{
				Symbol: "BTC/USD", Type: "update", Timestamp: at,
			})

			if signal.Measure(market) != nil {
				panic("pumpdump: measure failed")
			}

			return slices.Collect(market.MarketMeasurements("category"))
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

	Convey("Given a live book and a dirty trade tape", t, func() {
		reads := 0
		signal := NewSignal(context.Background(), countingBookSource{
			book: seededBook(), count: &reads,
		})
		market := types.NewSymbol("BTC/USD", nil)
		market.AppendLevel3(kraken.Level3Data{
			Symbol: "BTC/USD", Type: "update",
			Timestamp: time.Unix(1_700_002_700, 0).UTC(),
		})
		market.AppendTrade(
			pumpdumpTrade(4, "buy", 100, time.Unix(1_700_002_701, 0).UTC()),
		)

		err := signal.Measure(market)
		So(err, ShouldBeNil)
		measurements := slices.Collect(market.MarketMeasurements("category"))

		Convey("It should read the book exactly once per pass", func() {
			// A second read re-enters Book.Get while the first callback still
			// holds the manager read lock, deadlocking as soon as a level3
			// writer queues for the write lock.
			So(measurements, ShouldHaveLength, 1)
			So(reads, ShouldEqual, 1)
		})
	})
}

type countingBookSource struct {
	book  *spotbook.Book
	count *int
}

func (source countingBookSource) Book(_ string, read func(*spotbook.Book)) {
	*source.count++
	read(source.book)
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
