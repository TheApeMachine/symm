package response

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
)

func configurePaperWallet() {
	viper.Set("market.quote_currency", "EUR")
	viper.Set("trading.paper.wallet_eur", 200.0)
}

func TestBalances(t *testing.T) {
	configurePaperWallet()

	Convey("Given a fresh paper balances wallet", t, func() {
		balances := NewBalances(nil)

		Convey("It funds the configured quote currency", func() {
			So(balances.model.Asset[0].Asset, ShouldEqual, "EUR")
			So(balances.model.Asset[0].Balance, ShouldEqual, 200.0)
			So(balances.model.Asset[0].Wallets[0].Balance, ShouldEqual, 200.0)
		})
	})
}

func TestApplyFill(t *testing.T) {
	configurePaperWallet()

	Convey("Given a paper wallet funded with 200 EUR", t, func() {
		balances := NewBalances(nil)

		Convey("A buy debits quote (cost+fee) and credits base", func() {
			balances.ApplyFill("BTC/EUR", "buy", 0.001, 50000, 0.13, "ord-1")

			So(balanceOf(balances, "EUR"), ShouldAlmostEqual, 200-50-0.13, 1e-9)
			So(balanceOf(balances, "BTC"), ShouldAlmostEqual, 0.001, 1e-12)
		})

		Convey("A round trip leaves only the paid fees out of quote", func() {
			balances.ApplyFill("BTC/EUR", "buy", 0.001, 50000, 0.13, "ord-1")
			balances.ApplyFill("BTC/EUR", "sell", 0.001, 50000, 0.13, "ord-2")

			So(balanceOf(balances, "EUR"), ShouldAlmostEqual, 200-0.26, 1e-9)
			So(balanceOf(balances, "BTC"), ShouldAlmostEqual, 0, 1e-12)
		})

		Convey("A buy in a quote currency we do not hold is rejected", func() {
			// EUR/AUD spends AUD, which the wallet has none of.
			err := balances.ApplyFill("EUR/AUD", "buy", 100, 1.6, 0.4, "ord-1")

			So(err, ShouldEqual, ErrInsufficientFunds)
			So(balanceOf(balances, "EUR"), ShouldEqual, 200)
			So(balanceOf(balances, "AUD"), ShouldEqual, 0)
		})

		Convey("A buy exceeding available quote is rejected, wallet untouched", func() {
			err := balances.ApplyFill("BTC/EUR", "buy", 1, 50000, 0.13, "ord-1")

			So(err, ShouldEqual, ErrInsufficientFunds)
			So(balanceOf(balances, "EUR"), ShouldEqual, 200)
			So(balanceOf(balances, "BTC"), ShouldEqual, 0)
		})
	})
}

func TestApplyFillPublishesOpenCount(t *testing.T) {
	configurePaperWallet()

	Convey("Given a wallet wired to a ui broadcast group", t, func() {
		pool := qpool.NewQ(context.Background(), 1, 4, nil)
		ui := pool.CreateBroadcastGroup("ui", 10*time.Millisecond)
		sub := ui.Subscribe("test:ui", 16)

		// Drain the snapshot publish from construction.
		balances := NewBalances(ui)
		drainWallet(sub)

		Convey("A buy publishes balance and a non-zero open count", func() {
			balances.ApplyFill("BTC/EUR", "buy", 0.001, 50000, 0.13, "ord-1")
			frame := drainWallet(sub)

			So(frame["event"], ShouldEqual, "wallet")
			So(frame["open"], ShouldEqual, 1)
			So(frame["balance"], ShouldAlmostEqual, 200-50-0.13, 1e-9)
		})
	})
}

func balanceOf(balances *Balances, asset string) float64 {
	for _, row := range balances.model.Asset {
		if row.Asset == asset {
			return row.Balance
		}
	}

	return 0
}

func drainWallet(sub *qpool.Subscriber) map[string]any {
	timeout := time.After(2 * time.Second)

	for {
		select {
		case msg := <-sub.Incoming:
			frame, _ := msg.Value.(map[string]any)

			if frame["event"] == "wallet" {
				return frame
			}
		case <-timeout:
			return nil
		}
	}
}
