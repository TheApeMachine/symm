package types

import (
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestAppendTicker(t *testing.T) {
	Convey("Given ticker observations arriving across symbols", t, func() {
		thesis := NewThesis()
		base := time.Unix(1_700_005_000, 0).UTC()
		thesis.AppendTicker(kraken.TickerData{
			Symbol: "ETH/USD", Timestamp: base.Add(2 * time.Second),
		})
		thesis.AppendTicker(kraken.TickerData{
			Symbol: "BTC/USD", Timestamp: base,
		})
		thesis.AppendTicker(kraken.TickerData{
			Symbol: "BTC/USD", Timestamp: base.Add(time.Second),
		})

		Convey("It should retain every row and expose a detached time-ordered snapshot", func() {
			tickers := thesis.MarketTickers()
			So(tickers, ShouldHaveLength, 3)
			So(tickers[0].Symbol, ShouldEqual, "BTC/USD")
			So(tickers[0].Timestamp, ShouldResemble, base)
			So(tickers[1].Timestamp, ShouldResemble, base.Add(time.Second))
			So(tickers[2].Timestamp, ShouldResemble, base.Add(2*time.Second))

			tickers[0].Symbol = "changed"
			So(thesis.MarketTickers()[0].Symbol, ShouldEqual, "BTC/USD")
		})
	})

	Convey("Given concurrent ticker writers", t, func() {
		thesis := NewThesis()
		base := time.Unix(1_700_005_100, 0).UTC()
		var waitGroup sync.WaitGroup

		for index := range 128 {
			waitGroup.Add(1)

			go func() {
				defer waitGroup.Done()
				thesis.AppendTicker(kraken.TickerData{
					Symbol:    "BTC/USD",
					Timestamp: base.Add(time.Duration(index)),
				})
			}()
		}

		waitGroup.Wait()

		Convey("It should retain every append without corrupting the symbol buffer", func() {
			So(thesis.MarketTickers(), ShouldHaveLength, 128)
		})
	})
}

func TestAppendTrade(t *testing.T) {
	Convey("Given trades arriving outside timestamp order", t, func() {
		thesis := NewThesis()
		base := time.Unix(1_700_005_200, 0).UTC()
		thesis.AppendTrade(kraken.TradeData{
			Symbol: "BTC/USD", TradeID: 102, Timestamp: base.Add(time.Second),
		})
		thesis.AppendTrade(kraken.TradeData{
			Symbol: "ETH/USD", TradeID: 201, Timestamp: base,
		})
		thesis.AppendTrade(kraken.TradeData{
			Symbol: "BTC/USD", TradeID: 101, Timestamp: base,
		})

		Convey("It should expose the complete ordered execution tape", func() {
			trades := thesis.MarketTrades()
			So(trades, ShouldHaveLength, 3)
			So(trades[0].TradeID, ShouldEqual, 101)
			So(trades[1].TradeID, ShouldEqual, 201)
			So(trades[2].TradeID, ShouldEqual, 102)
		})
	})
}

func TestMarketCloseCycle(t *testing.T) {
	Convey("Given retained market history from a completed decision cycle", t, func() {
		thesis := NewThesis()
		at := time.Unix(1_700_005_300, 0).UTC()
		thesis.AppendTicker(kraken.TickerData{Symbol: "BTC/USD", Timestamp: at})
		thesis.AppendTrade(kraken.TradeData{
			Symbol: "BTC/USD", TradeID: 301, Timestamp: at,
		})
		closedAt := thesis.CloseCycle().At

		Convey("It should begin the next cycle with empty market buffers", func() {
			So(closedAt.IsZero(), ShouldBeFalse)
			So(thesis.MarketTickers(), ShouldBeEmpty)
			So(thesis.MarketTrades(), ShouldBeEmpty)
			So(thesis.MarketSymbols(), ShouldBeEmpty)
		})
	})
}

func BenchmarkAppendTicker(b *testing.B) {
	thesis := NewThesis()
	ticker := kraken.TickerData{
		Symbol: "BTC/USD", Timestamp: time.Unix(1_700_005_400, 0).UTC(),
	}
	b.ReportAllocs()

	for b.Loop() {
		thesis.AppendTicker(ticker)
	}
}

func BenchmarkAppendTrade(b *testing.B) {
	thesis := NewThesis()
	trade := kraken.TradeData{
		Symbol: "BTC/USD", TradeID: 401,
		Timestamp: time.Unix(1_700_005_500, 0).UTC(),
	}
	b.ReportAllocs()

	for b.Loop() {
		thesis.AppendTrade(trade)
	}
}
