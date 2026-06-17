package fluid

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	feed "github.com/theapemachine/symm/signal"
)

func TestSignalMarketFacts(testingTB *testing.T) {
	Convey("Given ticker and trade updates", testingTB, func() {
		signal := NewSignal(context.Background(), qpool.NewQ[any](context.Background(), 1, 2, nil))
		scope := "BTC/EUR"
		observed := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

		signal.Update(feed.TickerFeedArtifact(krakenmarket.TickerUpdates{{
			Symbol:    scope,
			Last:      100,
			Bid:       99.5,
			Ask:       100.5,
			Volume:    12,
			Timestamp: observed,
		}}))
		signal.Update(feed.TradeFeedArtifact(krakenmarket.TradeUpdates{{
			Symbol:    scope,
			Price:     100,
			Qty:       2,
			Side:      "buy",
			Timestamp: observed,
		}}))

		facts := signal.MarketFacts(scope)

		Convey("It should expose quote context for measurement enrichment", func() {
			So(facts.Price, ShouldEqual, 100)
			So(facts.Volume, ShouldBeGreaterThan, 0)
			So(facts.Spread, ShouldBeGreaterThan, 0)
			So(facts.ObservedAt, ShouldEqual, observed)
		})
	})
}
