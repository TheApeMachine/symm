package toxicity

import (
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestSeenTrade(t *testing.T) {
	Convey("Given an exact-once cursor for one toxicity symbol", t, func() {
		signal := &Signal{lastTrade: make(map[string]tradeCursor)}
		at := time.Unix(1_700_004_000, 0).UTC()
		first := kraken.TradeData{Symbol: "ALT/USD", TradeID: 81, Timestamp: at}
		second := kraken.TradeData{Symbol: "ALT/USD", TradeID: 82, Timestamp: at}
		regressed := kraken.TradeData{Symbol: "ALT/USD", TradeID: 83, Timestamp: at.Add(-time.Nanosecond)}

		Convey("It accepts distinct same-time IDs and rejects replay or regression", func() {
			So(signal.seenTrade(first), ShouldBeFalse)
			signal.commitTrade(first)
			So(signal.seenTrade(first), ShouldBeTrue)
			So(signal.seenTrade(second), ShouldBeFalse)
			signal.commitTrade(second)
			So(signal.seenTrade(second), ShouldBeTrue)
			So(signal.seenTrade(regressed), ShouldBeTrue)
		})
	})

	Convey("Given same-time toxicity trades without exchange IDs", t, func() {
		signal := &Signal{lastTrade: make(map[string]tradeCursor)}
		trade := kraken.TradeData{Symbol: "ALT/USD", Timestamp: time.Unix(1_700_004_100, 0).UTC()}
		signal.commitTrade(trade)

		Convey("It documents intrinsic zero-ID indistinguishability", func() {
			So(signal.seenTrade(trade), ShouldBeTrue)
		})
	})
}

func TestMeasure(t *testing.T) {
	Convey("Given a pre-touch, multiple executions, and a later post-touch", t, func() {
		signal := &Signal{
			previousTouch: make(map[string]touchSnapshot),
			pendingTrades: make(map[string]map[tradeIdentity]kraken.TradeData),
			lastTrade:     make(map[string]tradeCursor),
		}
		base := time.Unix(1_700_004_200, 0).UTC()
		preCut := types.NewThesis()
		preCut.Books.Store("BTC/USD", toxicityBook(100, 101, 10, 10, base))
		So(signal.Measure(preCut), ShouldBeEmpty)

		firstTrade := toxicityTrade(91, "sell", 100, 2, base.Add(time.Second))
		secondTrade := toxicityTrade(92, "buy", 101, 3, base.Add(2*time.Second))
		tradeCut := types.NewThesis()
		tradeCut.Books.Store("BTC/USD", toxicityBook(100, 101, 10, 10, base))
		tradeCut.Trades.Store(int64(91), firstTrade)
		tradeCut.Trades.Store(int64(92), secondTrade)

		Convey("It keeps both trades pending until a strict post-trade touch arrives", func() {
			So(signal.Measure(tradeCut), ShouldBeEmpty)
			So(signal.seenTrade(firstTrade), ShouldBeFalse)
			So(signal.seenTrade(secondTrade), ShouldBeFalse)

			postAt := base.Add(3 * time.Second)
			postCut := types.NewThesis()
			postCut.Books.Store("BTC/USD", toxicityBook(99, 101, 7, 6, postAt))
			measurements := signal.Measure(postCut)
			So(measurements, ShouldHaveLength, 1)
			measurement := measurements[0]
			So(measurement.ObservedFrom, ShouldResemble, base)
			So(measurement.At, ShouldResemble, postAt)
			So(measurement.Horizon, ShouldEqual, 3*time.Second)
			So(measurement.Validity.State, ShouldEqual, types.ValidityValid)
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
			So(signal.seenTrade(firstTrade), ShouldBeTrue)
			So(signal.seenTrade(secondTrade), ShouldBeTrue)
		})
	})

	Convey("Given only a pre-trade touch", t, func() {
		signal := &Signal{
			previousTouch: make(map[string]touchSnapshot),
			pendingTrades: make(map[string]map[tradeIdentity]kraken.TradeData),
			lastTrade:     make(map[string]tradeCursor),
		}
		base := time.Unix(1_700_004_300, 0).UTC()
		preCut := types.NewThesis()
		preCut.Books.Store("BTC/USD", toxicityBook(100, 101, 10, 10, base))
		So(signal.Measure(preCut), ShouldBeEmpty)
		trade := toxicityTrade(93, "buy", 101, 1, base.Add(time.Second))
		tradeCut := types.NewThesis()
		tradeCut.Books.Store("BTC/USD", toxicityBook(100, 101, 10, 10, base))
		tradeCut.Trades.Store(int64(93), trade)

		Convey("It cannot claim validity or consume the unresolved trade", func() {
			So(signal.Measure(tradeCut), ShouldBeEmpty)
			So(signal.seenTrade(trade), ShouldBeFalse)
			So(signal.pendingTrades["BTC/USD"], ShouldHaveLength, 1)
		})
	})
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
