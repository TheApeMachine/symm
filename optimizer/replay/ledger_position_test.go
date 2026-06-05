package replay

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestReplayShortEntry(t *testing.T) {
	Convey("Given a replay ledger with EUR funding", t, func() {
		costs := ReplayCosts{
			StartingCapital:  200,
			PositionFraction: 1,
			WalletCurrency:   "EUR",
			WalletBalances:   map[string]float64{"EUR": 200},
		}
		ledger := newReplayLedger(costs)

		Convey("A short entry profits when price falls", func() {
			ledger.openShort("BTC/EUR", 100, 0, 0, time.Time{})
			So(ledger.holding("BTC/EUR"), ShouldBeTrue)
			So(ledger.positions["BTC/EUR"].side, ShouldEqual, trading.Sell)

			ledger.applyStressed(
				perspectives.Act{Type: perspectives.ActionSettlePosition},
				eurRow("BTC/EUR", 90),
				nil,
			)

			So(ledger.holding("BTC/EUR"), ShouldBeFalse)
			So(ledger.realizedReturn(), ShouldAlmostEqual, 0.10, 1e-9)
		})
	})
}

func TestReplayMultiCurrencyWallets(t *testing.T) {
	Convey("Given separate EUR and USD wallets", t, func() {
		costs := ReplayCosts{
			PositionFraction: 0.5,
			WalletBalances: map[string]float64{
				"EUR": 200,
				"USD": 300,
			},
		}
		ledger := newReplayLedger(costs)

		ledger.openLong("BTC/EUR", 100, 0, 0, time.Time{})
		ledger.openLong("ETH/USD", 50, 0, 0, time.Time{})

		So(ledger.holding("BTC/EUR"), ShouldBeTrue)
		So(ledger.holding("ETH/USD"), ShouldBeTrue)
		So(ledger.walletCash("EUR"), ShouldEqual, 100)
		So(ledger.walletCash("USD"), ShouldEqual, 150)
	})
}
