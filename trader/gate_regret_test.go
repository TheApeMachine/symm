package trader

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/wallet"
)

func TestFlushOpenPositionPerformance(t *testing.T) {
	convey.Convey("Given an open replay position marked above entry", t, func() {
		crypto := newTestCrypto()
		openedAt := time.Now().Add(-time.Second)
		crypto.wallet.AddInventoryWithCost("BTC", 1, 100)
		crypto.wallet.BindPosition("BTC", wallet.PositionBinding{
			Source:      "perspective",
			Playbook:    "drive",
			EntryFeePct: 0.25,
			ExitFeePct:  0.40,
			PredictedAt: openedAt,
			DueAt:       openedAt.Add(time.Minute),
		})
		crypto.quotes.ingestTicker(market.TickerUpdate{
			Symbol: "BTC/EUR",
			Last:   101,
			Bid:    100.95,
			Ask:    101.05,
		})

		crypto.FlushOpenPositionPerformance()
		summary := crypto.PerformanceSummary()

		convey.Convey("It should count the final mark as a profitable closed sample", func() {
			convey.So(summary.ClosedTrades, convey.ShouldEqual, 1)
			convey.So(summary.ProfitableTrades, convey.ShouldEqual, 1)
		})
	})
}
