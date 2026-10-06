package broker

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestCashPrefersAvailableOverTotal(t *testing.T) {
	Convey("Cash sizes from Available when Total is larger (open holds)", t, func() {
		balance := &Balance{Quote: "USD"}
		err := balance.UpdateWallet(&kraken.Balance{
			Type: "snapshot",
			Data: []kraken.BalanceData{{
				Asset:     "USD",
				Balance:   decimal.NewFromFloat64(1000),
				Available: decimal.NewFromFloat64(0.19331402),
				Reserved:  decimal.NewFromFloat64(999.80668598),
			}},
		})
		So(err, ShouldBeNil)

		cash := balance.Cash()
		So(cash, ShouldNotBeNil)
		So(cash.Cmp(decimal.NewFromFloat64(0.19331402)), ShouldEqual, 0)
	})
}

func TestCashFallsBackToBalanceWhenAvailableAbsent(t *testing.T) {
	Convey("Cash uses Balance when Available is nil", t, func() {
		balance := &Balance{Quote: "USD"}
		err := balance.UpdateWallet(&kraken.Balance{
			Type: "snapshot",
			Data: []kraken.BalanceData{{
				Asset:   "USD",
				Balance: decimal.NewFromFloat64(42),
			}},
		})
		So(err, ShouldBeNil)

		cash := balance.Cash()
		So(cash, ShouldNotBeNil)
		So(cash.Cmp(decimal.NewFromFloat64(42)), ShouldEqual, 0)
	})
}

func TestBalance_UpdateWallet(t *testing.T) {
	Convey("Given a balance holding the venue's last trade balance", t, func() {
		balance := &Balance{Quote: "USD"}
		balance.snapshot.Store(newAccountSnapshot("USD", nil, &kraken.TradeBalanceResult{
			Equity:        decimal.NewFromFloat64(199.16),
			UnrealizedPnL: decimal.NewFromFloat64(-0.84),
		}))

		Convey("When a streamed wallet frame arrives", func() {
			err := balance.UpdateWallet(&kraken.Balance{
				Type: "snapshot",
				Data: []kraken.BalanceData{{
					Asset:     "USD",
					Balance:   decimal.NewFromFloat64(150),
					Available: decimal.NewFromFloat64(150),
				}},
			})
			So(err, ShouldBeNil)

			Convey("It updates cash and keeps the venue's equity and unrealized PnL", func() {
				frame := balance.EquityWire()
				So(frame, ShouldNotBeNil)
				So(frame.Cash, ShouldEqual, decimal.NewFromFloat64(150).String())
				So(frame.Equity, ShouldEqual, decimal.NewFromFloat64(199.16).String())
				So(frame.Unrealized, ShouldEqual, decimal.NewFromFloat64(-0.84).String())
			})
		})
	})
}

func TestBalance_UpdateWalletLedgerUpdate(t *testing.T) {
	Convey("Given a balance holding a wallet snapshot", t, func() {
		balance := &Balance{Quote: "USD"}
		So(balance.UpdateWallet(&kraken.Balance{
			Type: "snapshot",
			Data: []kraken.BalanceData{
				{Asset: "USD", Balance: decimal.NewFromFloat64(500), Available: decimal.NewFromFloat64(400)},
				{Asset: "BTC", Balance: decimal.NewFromFloat64(0.5)},
				{Asset: "ETH", Balance: decimal.NewFromFloat64(2)},
			},
		}), ShouldBeNil)

		Convey("When a ledger update for one non-quote asset arrives", func() {
			err := balance.UpdateWallet(&kraken.Balance{
				Type: "update",
				Data: []kraken.BalanceData{
					{Asset: "BTC", Balance: decimal.NewFromFloat64(0.4)},
					{Asset: "BTC", Balance: decimal.NewFromFloat64(0.3)},
				},
			})
			So(err, ShouldBeNil)

			Convey("Only that asset changes, to the last total, and cash is kept", func() {
				assets := balance.Assets()
				So(assets, ShouldHaveLength, 3)
				So(assets["BTC"].Cmp(decimal.NewFromFloat64(0.3)), ShouldEqual, 0)
				So(assets["ETH"].Cmp(decimal.NewFromFloat64(2)), ShouldEqual, 0)
				So(balance.Cash().Cmp(decimal.NewFromFloat64(400)), ShouldEqual, 0)
			})
		})

		Convey("When a ledger update for the quote asset arrives", func() {
			So(balance.UpdateWallet(&kraken.Balance{
				Type: "update",
				Data: []kraken.BalanceData{{Asset: "USD", Balance: decimal.NewFromFloat64(450)}},
			}), ShouldBeNil)

			Convey("Cash moves to the new total (the ledger carries no hold split)", func() {
				So(balance.Cash().Cmp(decimal.NewFromFloat64(450)), ShouldEqual, 0)
				So(balance.Assets()["BTC"].Cmp(decimal.NewFromFloat64(0.5)), ShouldEqual, 0)
			})
		})

		Convey("When a ledger update introduces a new asset", func() {
			So(balance.UpdateWallet(&kraken.Balance{
				Type: "update",
				Data: []kraken.BalanceData{{Asset: "SOL", Balance: decimal.NewFromFloat64(10)}},
			}), ShouldBeNil)

			Convey("It is added alongside the held assets", func() {
				So(balance.Assets(), ShouldHaveLength, 4)
			})
		})

		Convey("A malformed update is rejected and the wallet is untouched", func() {
			So(balance.UpdateWallet(&kraken.Balance{
				Type: "update",
				Data: []kraken.BalanceData{{Asset: "BTC"}},
			}), ShouldNotBeNil)
			So(balance.Assets()["BTC"].Cmp(decimal.NewFromFloat64(0.5)), ShouldEqual, 0)
		})

		Convey("An unknown frame type is rejected", func() {
			So(balance.UpdateWallet(&kraken.Balance{Type: "weird"}), ShouldNotBeNil)
			So(balance.Assets(), ShouldHaveLength, 3)
		})
	})

	Convey("Given a balance with no held wallet", t, func() {
		balance := &Balance{Quote: "USD"}

		Convey("An update cannot be merged and is an error", func() {
			So(balance.UpdateWallet(&kraken.Balance{
				Type: "update",
				Data: []kraken.BalanceData{{Asset: "BTC", Balance: decimal.NewFromFloat64(1)}},
			}), ShouldNotBeNil)
			So(balance.Snapshot(), ShouldBeNil)
		})
	})
}
