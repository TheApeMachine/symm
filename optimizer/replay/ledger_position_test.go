package replay

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
)

func TestReplayShortEntry(t *testing.T) {
	Convey("Given a replay ledger with EUR funding", t, func() {
		testconfig.Load(t)
		costs := ReplayCosts{
			StartingCapital:  200,
			PositionFraction: 1,
			WalletCurrency:   "EUR",
			WalletBalances:   map[string]float64{"EUR": 200},
		}
		ledger := newReplayLedger(costs)

		Convey("A short entry profits when price falls", func() {
			ledger.openShort("BTC/EUR", 100, 0, time.Time{})
			So(ledger.holding("BTC/EUR"), ShouldBeTrue)
			So(ledger.positions["BTC/EUR"].side, ShouldEqual, trading.Sell)

			ledger.applyStressed(
				reasoning.Act{Type: reasoning.ActionSettlePosition},
				eurRow("BTC/EUR", 90),
				nil,
				0,
			)

			So(ledger.holding("BTC/EUR"), ShouldBeFalse)
			So(ledger.realizedReturn(), ShouldAlmostEqual, 0.10, 1e-3)
		})
	})
}

func TestReplayEntryRejectsRaisedMinimumAboveWallet(t *testing.T) {
	Convey("Given instrument rules that raise entry cost above cash", t, func() {
		rules := broker.NewInstrumentRulesCache(t.Context())
		rules.InstallPairForTest(market.InstrumentPair{
			Symbol:       "FXS/EUR",
			QtyIncrement: 0.00000001,
			QtyMin:       12,
			CostMin:      10,
		})

		costs := ReplayCosts{
			StartingCapital:  5,
			PositionFraction: 1,
			WalletCurrency:   "EUR",
			WalletBalances:   map[string]float64{"EUR": 5},
			InstrumentRules:  rules,
		}
		ledger := newReplayLedger(costs)
		measurement := TradeableRow("FXS/EUR", 4.36, time.Unix(1_700_000_000, 0))

		ledger.openEntry(
			"FXS/EUR",
			trading.Buy,
			reasoning.Act{Type: reasoning.ActionMarket},
			measurement,
			nil,
			0,
			measurement.At,
			0,
		)

		Convey("It should refuse the entry instead of overspending", func() {
			So(ledger.holding("FXS/EUR"), ShouldBeFalse)
			So(ledger.fundBlocked, ShouldEqual, 1)
			So(ledger.walletCash("EUR"), ShouldEqual, 5)
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

		ledger.openLong("BTC/EUR", 100, 0, time.Time{})
		ledger.openLong("ETH/USD", 50, 0, time.Time{})

		So(ledger.holding("BTC/EUR"), ShouldBeTrue)
		So(ledger.holding("ETH/USD"), ShouldBeTrue)
		So(ledger.walletCash("EUR"), ShouldEqual, 100)
		So(ledger.walletCash("USD"), ShouldEqual, 150)
	})
}
