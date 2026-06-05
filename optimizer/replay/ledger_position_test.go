package replay

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestReplayLedgerFundableSymbol(t *testing.T) {
	Convey("Given a replay ledger funded in EUR", t, func() {
		costs := triggerTestCosts()
		costs.WalletCurrency = "EUR"
		ledger := newReplayLedger(costs)

		Convey("It should only fund pairs with the wallet quote currency", func() {
			So(ledger.fundableSymbol("BTC/EUR"), ShouldBeTrue)
			So(ledger.fundableSymbol("ETH/BTC"), ShouldBeFalse)
		})

		Convey("It should treat unparseable symbols as fundable", func() {
			So(ledger.fundableSymbol("BTC"), ShouldBeTrue)
		})
	})
}

func TestReplayLedgerOpenLongFundBlocked(t *testing.T) {
	Convey("Given a ledger fully deployed in one position", t, func() {
		ledger := newReplayLedger(triggerTestCosts())
		ledger.openLong("BTC/EUR", 100, 0, 0, time.Time{})

		Convey("It should count a second entry as fund-blocked", func() {
			ledger.openLong("ETH/EUR", 50, 0, 0, time.Time{})

			So(ledger.fundBlocked, ShouldEqual, 1)
			So(ledger.holding("ETH/EUR"), ShouldBeFalse)
		})
	})
}

func TestReplayLedgerPreviewClosePnL(t *testing.T) {
	Convey("Given an open long", t, func() {
		ledger := newReplayLedger(triggerTestCosts())
		ledger.openLong("BTC/EUR", 100, 0, 0, time.Time{})

		Convey("It should preview the fractional return at the exit fill", func() {
			preview := ledger.previewClosePnL("BTC/EUR", 102, 0)

			So(preview, ShouldAlmostEqual, 0.02, 1e-9)
		})

		Convey("It should return zero without an open position", func() {
			So(ledger.previewClosePnL("ETH/EUR", 102, 0), ShouldEqual, 0)
		})
	})
}

func TestReplayLedgerObservations(t *testing.T) {
	Convey("Given a replay ledger position state", t, func() {
		ledger := newReplayLedger(triggerTestCosts())

		Convey("It should expose not-holding when flat", func() {
			observations := ledger.observations("BTC/EUR")

			So(observations[perspectives.ObservationNotHolding], ShouldEqual, 1)
		})

		Convey("It should expose holding after entry", func() {
			ledger.openLong("BTC/EUR", 100, 0, 0, time.Time{})

			observations := ledger.observations("BTC/EUR")

			So(observations[perspectives.ObservationHolding], ShouldEqual, 1)
		})
	})
}

func TestReplayLedgerMetrics(t *testing.T) {
	Convey("Given an open long with a fresh quote", t, func() {
		ledger := newReplayLedger(triggerTestCosts())
		ledger.openLong("BTC/EUR", 100, 0, 0, time.Time{})

		Convey("It should publish last and unrealized return", func() {
			metrics := ledger.metrics(perspectives.Measurement{
				Symbol: "BTC/EUR",
				Last:   102,
			})

			So(metrics["last"], ShouldEqual, 102)
			So(metrics["unrealized_return"], ShouldAlmostEqual, 2, 1e-9)
		})
	})
}

func TestHoldoutDecay(t *testing.T) {
	Convey("Given train and test per-trade returns", t, func() {
		Convey("It should return the relative decay fraction", func() {
			So(HoldoutDecay(0.10, 0.05), ShouldAlmostEqual, 0.5, 1e-9)
		})

		Convey("It should return positive infinity when train is non-positive", func() {
			So(HoldoutDecay(0, 0.05), ShouldEqual, math.Inf(1))
		})
	})
}

func BenchmarkReplayLedgerOpenLong(b *testing.B) {
	ledger := newReplayLedger(triggerTestCosts())

	for b.Loop() {
		ledger.reset(triggerTestCosts())
		ledger.openLong("BTC/EUR", 100, 0, 0, time.Time{})
	}
}
