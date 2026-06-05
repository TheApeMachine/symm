package replay

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
)

// triggerTestCosts strips fees and slippage so realized P&L equals the raw
// trigger arithmetic.
func triggerTestCosts() ReplayCosts {
	return ReplayCosts{
		StopLossPct:                0.02, // exit 2% below entry
		TakeProfitPct:              0.03, // exit 3% above entry
		TrailingVolatilityMultiple: 3,
		StartingCapital:            100, // €100 account so realizedReturn is P&L / 100
		PositionFraction:           1,   // deploy the whole balance per entry
		WalletCurrency:             "EUR",
	}
}

func btcRow(last float64) perspectives.Measurement {
	return perspectives.Measurement{Symbol: "BTC/EUR", Last: last}
}

func TestReplayTriggerExits(t *testing.T) {
	Convey("Given an open long with no fees or slippage", t, func() {
		Convey("A stop-loss rests until price falls to the trigger, then realizes the loss", func() {
			ledger := newReplayLedger(triggerTestCosts())
			ledger.openLong("BTC/EUR", 100, 0, time.Time{})
			ledger.armTrigger("BTC/EUR", perspectives.Act{Type: perspectives.ActionStopLoss})

			ledger.checkTriggers(btcRow(99)) // above the 98 trigger — stays open
			So(ledger.holding("BTC/EUR"), ShouldBeTrue)
			So(ledger.closedTrades, ShouldEqual, 0)

			ledger.checkTriggers(btcRow(98)) // touches the trigger — fills at 98
			So(ledger.holding("BTC/EUR"), ShouldBeFalse)
			So(ledger.closedTrades, ShouldEqual, 1)
			So(ledger.realizedReturn(), ShouldAlmostEqual, -0.02, 1e-9)
		})

		Convey("A market stop-loss eats a downside gap-through", func() {
			ledger := newReplayLedger(triggerTestCosts())
			ledger.openLong("BTC/EUR", 100, 0, time.Time{})
			ledger.armTrigger("BTC/EUR", perspectives.Act{Type: perspectives.ActionStopLoss})

			ledger.checkTriggers(btcRow(97)) // gaps below the 98 trigger — fills at 97
			So(ledger.realizedReturn(), ShouldAlmostEqual, -0.03, 1e-9)
		})

		Convey("A stop-loss-LIMIT fills at its trigger level (no gap-through), paying maker fee", func() {
			costs := triggerTestCosts()
			costs.MakerFeePct = 0.001 // 0.1%
			ledger := newReplayLedger(costs)
			ledger.openLong("BTC/EUR", 100, 0, time.Time{})
			ledger.armTrigger("BTC/EUR", perspectives.Act{Type: perspectives.ActionStopLossLimit})

			ledger.checkTriggers(btcRow(97)) // gaps through, but the resting limit fills at 98
			// fill 98 less the maker fee on the exit proceeds, over €100 capital.
			So(ledger.realizedReturn(), ShouldAlmostEqual, (98.0*(1-0.001)-100.0)/100.0, 1e-9)
		})

		Convey("A take-profit rests until price rises to the target", func() {
			ledger := newReplayLedger(triggerTestCosts())
			ledger.openLong("BTC/EUR", 100, 0, time.Time{})
			ledger.armTrigger("BTC/EUR", perspectives.Act{Type: perspectives.ActionTakeProfit})

			ledger.checkTriggers(btcRow(102)) // below the 103 target — stays open
			So(ledger.holding("BTC/EUR"), ShouldBeTrue)

			ledger.checkTriggers(btcRow(103)) // hits the target — fills at 103
			So(ledger.holding("BTC/EUR"), ShouldBeFalse)
			So(ledger.realizedReturn(), ShouldAlmostEqual, 0.03, 1e-9)
		})

		Convey("A trailing stop ratchets with the peak and locks in the run-up", func() {
			ledger := newReplayLedger(triggerTestCosts())
			ledger.openLong("BTC/EUR", 100, 0, time.Time{})

			ledger.checkTriggers(btcRow(101))
			ledger.checkTriggers(btcRow(102))
			ledger.armTrigger("BTC/EUR", perspectives.Act{Type: perspectives.ActionTrailingStop})
			So(ledger.holding("BTC/EUR"), ShouldBeTrue)

			ledger.checkTriggers(btcRow(101.9))
			So(ledger.holding("BTC/EUR"), ShouldBeFalse)
			So(ledger.realizedReturn(), ShouldAlmostEqual, 0.019, 1e-9)
		})

		Convey("settle_position still closes immediately at the current price", func() {
			ledger := newReplayLedger(triggerTestCosts())
			ledger.openLong("BTC/EUR", 100, 0, time.Time{})
			ledger.applyStressed(perspectives.Act{Type: perspectives.ActionSettlePosition}, btcRow(101), nil)

			So(ledger.holding("BTC/EUR"), ShouldBeFalse)
			So(ledger.realizedReturn(), ShouldAlmostEqual, 0.01, 1e-9)
		})

		Convey("An armed trigger that never breaches leaves the position open (no phantom close)", func() {
			ledger := newReplayLedger(triggerTestCosts())
			ledger.openLong("BTC/EUR", 100, 0, time.Time{})
			ledger.armTrigger("BTC/EUR", perspectives.Act{Type: perspectives.ActionStopLoss})

			ledger.checkTriggers(btcRow(105))
			ledger.checkTriggers(btcRow(101))

			So(ledger.holding("BTC/EUR"), ShouldBeTrue)
			So(ledger.closedTrades, ShouldEqual, 0)
		})
	})
}

func TestMakerEntryMissed(t *testing.T) {
	Convey("Given a pending entry awaiting its execution tick", t, func() {
		Convey("A maker (limit) buy misses when price runs above the posted level", func() {
			So(makerEntryMissed(perspectives.ActionLimit, trading.Buy, 100, 100.5), ShouldBeTrue)
		})

		Convey("A maker (limit) buy fills when price comes back to the post", func() {
			So(makerEntryMissed(perspectives.ActionLimit, trading.Buy, 100, 99.8), ShouldBeFalse)
			So(makerEntryMissed(perspectives.ActionLimit, trading.Buy, 100, 100), ShouldBeFalse)
		})

		Convey("A taker (market) buy always fills regardless of drift", func() {
			So(makerEntryMissed(perspectives.ActionMarket, trading.Buy, 100, 105), ShouldBeFalse)
		})

		Convey("Exit actions are never treated as maker-entry misses", func() {
			So(makerEntryMissed(perspectives.ActionSettlePosition, trading.Buy, 100, 105), ShouldBeFalse)
			So(makerEntryMissed(perspectives.ActionStopLoss, trading.Buy, 100, 105), ShouldBeFalse)
		})
	})
}

func BenchmarkCheckTriggers(b *testing.B) {
	ledger := newReplayLedger(triggerTestCosts())
	ledger.openLong("BTC/EUR", 100, 0, time.Time{})
	ledger.armTrigger("BTC/EUR", perspectives.Act{Type: perspectives.ActionStopLoss})
	row := btcRow(99)

	for b.Loop() {
		ledger.checkTriggers(row)
	}
}
