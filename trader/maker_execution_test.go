package trader

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestCryptoMakerPaperWaitsForQueue(t *testing.T) {
	convey.Convey("Given a resting paper maker bid", t, func() {
		originalUseMaker := config.System.UseMakerEntries
		originalFallback := config.System.ExecutionMakerFallbackTicks
		config.System.UseMakerEntries = true
		config.System.ExecutionMakerFallbackTicks = 1000
		t.Cleanup(func() {
			config.System.UseMakerEntries = originalUseMaker
			config.System.ExecutionMakerFallbackTicks = originalFallback
			config.SyncRuntime()
		})
		config.SyncRuntime()

		crypto := newTestCrypto()
		crypto.runtime = config.Runtime
		crypto.quotes.ingestTicker(market.TickerUpdate{
			Symbol: "BTC/EUR",
			Last:   100,
			Bid:    100,
			Ask:    101,
		})
		crypto.quotes.ingestBook(market.BookUpdate{
			Symbol: "BTC/EUR",
			Bids:   []market.BookLevel{{Price: 100, Qty: 1}},
		})

		opportunity := opportunity{
			Symbol:  "BTC/EUR",
			Score:   2,
			Names:   []string{string(perspectives.PlaybookDrive)},
			Trigger: traderMeasurement("BTC/EUR", perspectives.SourceCVD, perspectives.CategoryAggressiveDrive, 2),
		}

		err := crypto.submitMakerEntryPaper(broker.Maker{
			Symbol:     "BTC/EUR",
			LimitPrice: 100,
			Notional:   10,
			FeePct:     0.25,
		}, crypto.quotes.snapshot("BTC/EUR", 100), opportunity, string(perspectives.PlaybookDrive), 10, 0.40)

		convey.Convey("It should keep cash reserved before sell-aggressor turnover", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(crypto.wallet.ReservedEUR, convey.ShouldEqual, 10)
			convey.So(crypto.makers.HasPending("BTC/EUR"), convey.ShouldBeTrue)
		})

		crypto.makers.observeTrade(market.TradeUpdate{
			Symbol:    "BTC/EUR",
			Side:      "buy",
			Price:     101,
			Qty:       2,
			Timestamp: time.Now(),
		})
		crypto.tryPaperMakerFills()
		drainOrderEvents(crypto)

		convey.Convey("It should ignore buy-aggressor trades", func() {
			convey.So(crypto.wallet.ReservedEUR, convey.ShouldEqual, 10)
			_, held := crypto.wallet.PositionBindingFor("BTC")
			convey.So(held, convey.ShouldBeFalse)
		})

		crypto.makers.observeTrade(market.TradeUpdate{
			Symbol:    "BTC/EUR",
			Side:      "sell",
			Price:     100,
			Qty:       1.15,
			Timestamp: time.Now(),
		})
		crypto.tryPaperMakerFills()
		drainOrderEvents(crypto)

		convey.Convey("It should fill once sell-aggressor volume clears the queue", func() {
			convey.So(crypto.wallet.ReservedEUR, convey.ShouldEqual, 0)
			binding, held := crypto.wallet.PositionBindingFor("BTC")
			convey.So(held, convey.ShouldBeTrue)
			convey.So(binding.EntryFeePct, convey.ShouldEqual, 0.25)
			convey.So(binding.ExitFeePct, convey.ShouldEqual, 0.40)
			convey.So(crypto.makers.HasPending("BTC/EUR"), convey.ShouldBeFalse)
		})
	})
}

func TestCryptoMakerPaperFallbackToTaker(t *testing.T) {
	convey.Convey("Given a maker bid that never fills", t, func() {
		originalUseMaker := config.System.UseMakerEntries
		originalFallback := config.System.ExecutionMakerFallbackTicks
		originalLatency := config.System.PaperOrderLatency
		originalSlippage := config.System.MaxEntrySlippageBPS
		config.System.UseMakerEntries = true
		config.System.ExecutionMakerFallbackTicks = 1
		config.System.PaperOrderLatency = 0
		config.System.MaxEntrySlippageBPS = 200
		t.Cleanup(func() {
			config.System.UseMakerEntries = originalUseMaker
			config.System.ExecutionMakerFallbackTicks = originalFallback
			config.System.PaperOrderLatency = originalLatency
			config.System.MaxEntrySlippageBPS = originalSlippage
			config.SyncRuntime()
		})
		config.SyncRuntime()

		crypto := newTestCrypto()
		crypto.runtime = config.Runtime
		crypto.quotes.ingestTicker(market.TickerUpdate{
			Symbol: "ETH/EUR",
			Last:   50,
			Bid:    50,
			Ask:    50.5,
		})
		crypto.quotes.ingestBook(market.BookUpdate{
			Symbol: "ETH/EUR",
			Bids:   []market.BookLevel{{Price: 50, Qty: 2}},
			Asks:   []market.BookLevel{{Price: 50.5, Qty: 2}},
		})

		opportunity := opportunity{
			Symbol: "ETH/EUR",
			Score:  4,
			Names:  []string{string(perspectives.PlaybookDrive)},
		}

		err := crypto.submitMakerEntryPaper(broker.Maker{
			Symbol:     "ETH/EUR",
			LimitPrice: 50,
			Notional:   10,
			FeePct:     0.25,
		}, crypto.quotes.snapshot("ETH/EUR", 50), opportunity, string(perspectives.PlaybookDrive), 8, 0.40)

		convey.So(err, convey.ShouldBeNil)

		crypto.advanceMakerFallback()
		drainOrderEvents(crypto)

		convey.Convey("It should release the maker and submit a taker fallback", func() {
			convey.So(crypto.makers.HasPending("ETH/EUR"), convey.ShouldBeFalse)
			_, held := crypto.wallet.PositionBindingFor("ETH")
			convey.So(held, convey.ShouldBeTrue)
		})
	})
}

func BenchmarkMakerDeskObserveTrade(b *testing.B) {
	desk := newMakerDesk()
	postedAt := time.Now()
	desk.track(&restingMakerEntry{
		clOrdID: "c1",
		symbol:  "BTC/EUR",
		tracker: broker.NewMakerQueueTracker("BTC/EUR", 100, postedAt, []market.BookLevel{
			{Price: 100, Qty: 1},
		}),
	})
	trade := market.TradeUpdate{
		Symbol:    "BTC/EUR",
		Side:      "sell",
		Price:     99.5,
		Qty:       0.1,
		Timestamp: postedAt.Add(time.Millisecond),
	}

	b.ReportAllocs()

	for b.Loop() {
		desk.observeTrade(trade)
	}
}
