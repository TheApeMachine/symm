package replay

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func eurRow(symbol string, last float64) perspectives.Measurement {
	return perspectives.Measurement{Symbol: symbol, Last: last}
}

func TestReplayCapitalConstraint(t *testing.T) {
	Convey("Given a €200 single-currency wallet", t, func() {
		costs := ReplayCosts{
			StartingCapital:  200,
			PositionFraction: 1, // deploy the whole balance per entry
			WalletCurrency:   "EUR",
		}

		Convey("Full deployment funds one position; a concurrent entry is unfunded", func() {
			ledger := newReplayLedger(costs)

			ledger.openLong("BTC/EUR", 100, 0, 0, time.Time{}) // spends the whole €200
			ledger.openLong("ETH/EUR", 50, 0, 0, time.Time{})  // no cash left — skipped, exactly as live

			So(ledger.holding("BTC/EUR"), ShouldBeTrue)
			So(ledger.holding("ETH/EUR"), ShouldBeFalse)
		})

		Convey("A pair quoted in another currency the wallet cannot pay is never opened", func() {
			ledger := newReplayLedger(costs)

			ledger.openLong("ETH/BTC", 100, 0, 0, time.Time{}) // quote is BTC; the EUR wallet can't fund it

			So(ledger.holding("ETH/BTC"), ShouldBeFalse)
			So(ledger.cash, ShouldEqual, 200) // untouched
		})

		Convey("A fractional position size lets capital fund several entries", func() {
			half := costs
			half.PositionFraction = 0.5
			ledger := newReplayLedger(half)

			ledger.openLong("BTC/EUR", 100, 0, 0, time.Time{}) // €100 deployed, €100 left
			ledger.openLong("ETH/EUR", 50, 0, 0, time.Time{})  // €100 deployed, €0 left

			So(ledger.holding("BTC/EUR"), ShouldBeTrue)
			So(ledger.holding("ETH/EUR"), ShouldBeTrue)
		})

		Convey("Each entry deploys a fraction of the capital base, not of remaining cash", func() {
			fractional := costs
			fractional.PositionFraction = 0.1
			fractional.StartingCapital = 200
			ledger := newReplayLedger(fractional)
			ledger.cash = 1000 // abundant cash must not inflate slot size

			ledger.openLong("BTC/EUR", 100, 0, 0, time.Time{})

			position := ledger.positions["BTC/EUR"]
			So(position.cost, ShouldAlmostEqual, 20, 1e-9) // 0.1 * 200 base
			So(position.quantity, ShouldAlmostEqual, 0.2, 1e-9)
		})

		Convey("realizedReturn is P&L as a fraction of the €200 account", func() {
			ledger := newReplayLedger(costs)

			ledger.openLong("BTC/EUR", 100, 0, 0, time.Time{}) // qty 2, cost €200
			ledger.applyStressed(perspectives.Act{Type: perspectives.ActionSettlePosition}, eurRow("BTC/EUR", 110), nil)

			// +10% price move on a fully-deployed account => +10% on capital.
			So(ledger.realizedReturn(), ShouldAlmostEqual, 0.10, 1e-9)
			So(ledger.cash, ShouldAlmostEqual, 220, 1e-9)
		})

		Convey("Freeing capital on exit lets the next entry fund again", func() {
			ledger := newReplayLedger(costs)
			ledger.reentryTickCooldown = 0

			ledger.openLong("BTC/EUR", 100, 0, 0, time.Time{})
			ledger.openLong("ETH/EUR", 50, 0, 0, time.Time{}) // unfunded while BTC/EUR holds
			So(ledger.holding("ETH/EUR"), ShouldBeFalse)

			ledger.applyStressed(perspectives.Act{Type: perspectives.ActionSettlePosition}, eurRow("BTC/EUR", 100), nil)
			ledger.openLong("ETH/EUR", 50, 0, 0, time.Time{}) // cash freed — now it funds
			So(ledger.holding("ETH/EUR"), ShouldBeTrue)
		})
	})
}
