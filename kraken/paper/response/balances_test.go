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
		balances := NewBalances(nil, nil, NewIdentifier())

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
		balances := NewBalances(nil, nil, NewIdentifier())

		Convey("A buy debits quote (cost+fee) and credits base", func() {
			balances.ApplyFill("BTC/EUR", "buy", 0.001, 50000, 0.13, "exec-1")

			So(balanceOf(balances, "EUR"), ShouldAlmostEqual, 200-50-0.13, 1e-9)
			So(balanceOf(balances, "BTC"), ShouldAlmostEqual, 0.001, 1e-12)
		})

		Convey("A round trip leaves only the paid fees out of quote", func() {
			balances.ApplyFill("BTC/EUR", "buy", 0.001, 50000, 0.13, "exec-1")
			balances.ApplyFill("BTC/EUR", "sell", 0.001, 50000, 0.13, "exec-2")

			So(balanceOf(balances, "EUR"), ShouldAlmostEqual, 200-0.26, 1e-9)
			So(balanceOf(balances, "BTC"), ShouldAlmostEqual, 0, 1e-12)
		})

		Convey("A buy in a quote currency we do not hold is rejected", func() {
			_, err := balances.ApplyFill("EUR/AUD", "buy", 100, 1.6, 0.4, "exec-1")

			So(err, ShouldEqual, ErrInsufficientFunds)
			So(balanceOf(balances, "EUR"), ShouldEqual, 200)
			So(balanceOf(balances, "AUD"), ShouldEqual, 0)
		})

		Convey("A buy exceeding available quote is rejected, wallet untouched", func() {
			_, err := balances.ApplyFill("BTC/EUR", "buy", 1, 50000, 0.13, "exec-1")

			So(err, ShouldEqual, ErrInsufficientFunds)
			So(balanceOf(balances, "EUR"), ShouldEqual, 200)
			So(balanceOf(balances, "BTC"), ShouldEqual, 0)
		})
	})
}

func TestApplyFillPublishesOpenCount(t *testing.T) {
	configurePaperWallet()

	Convey("Given a wallet wired to a ui broadcast group", t, func() {
		ui, err := qpool.NewBroadcastGroup(context.Background(), "ui", 10*time.Millisecond)
		if err != nil {
			t.Fatal("expected ui broadcast group")
		}
		sub := ui.Subscribe("test:ui", 16)

		balances := NewBalances(nil, ui, NewIdentifier())
		drainWallet(sub)

		Convey("A buy publishes balance and a non-zero open count", func() {
			balances.ApplyFill("BTC/EUR", "buy", 0.001, 50000, 0.13, "exec-1")
			frame := drainWallet(sub)

			So(frame["event"], ShouldEqual, "wallet")
			So(frame["open"], ShouldEqual, 1)
			So(frame["balance"], ShouldAlmostEqual, 200-50-0.13, 1e-9)
			So(frame["Balance"], ShouldAlmostEqual, 200-50-0.13, 1e-9)

			inventory, ok := frame["Inventory"].(map[string]float64)
			So(ok, ShouldBeTrue)
			So(inventory["BTC"], ShouldAlmostEqual, 0.001, 1e-12)
		})
	})
}

func TestBalancesSend(t *testing.T) {
	configurePaperWallet()

	Convey("Given a wallet wired to raw and ui groups", t, func() {
		raw, err := qpool.NewBroadcastGroup(context.Background(), "raw", 10*time.Millisecond)
		if err != nil {
			t.Fatal("expected raw broadcast group")
		}
		ui, err := qpool.NewBroadcastGroup(context.Background(), "ui", 10*time.Millisecond)
		if err != nil {
			t.Fatal("expected ui broadcast group")
		}
		balances := NewBalances(raw, ui, NewIdentifier())
		uiSub := ui.Subscribe("test:ui:subscribe", 16)
		rawSub := raw.Subscribe("test:raw:subscribe", 16)

		drainWallet(uiSub)

		Convey("A subscribe request republishes the current wallet and snapshot", func() {
			out := balances.Send(&qpool.QValue[any]{
				Value: map[string]any{"method": "subscribe"},
			})
			frame := drainWallet(uiSub)
			rawFrame := drainRawBalances(rawSub)

			So(out["success"], ShouldBeTrue)
			So(frame["event"], ShouldEqual, "wallet")
			So(frame["balance"], ShouldEqual, 200.0)
			So(rawFrame["channel"], ShouldEqual, "balances")
			So(rawFrame["type"], ShouldEqual, "snapshot")
		})
	})
}

func TestApplyFillRealizedPnL(t *testing.T) {
	configurePaperWallet()

	Convey("Given a paper wallet funded with 200 EUR", t, func() {
		balances := NewBalances(nil, nil, NewIdentifier())

		Convey("A winning round trip realizes net P&L equal to the wallet's gain (fees included)", func() {
			balances.ApplyFill("ETH/EUR", "buy", 1, 100, 0.4, "exec-1")   // fee-inclusive basis 100.4
			balances.ApplyFill("ETH/EUR", "sell", 1, 110, 0.44, "exec-2") // proceeds 109.56

			// realized = (110 - 0.44) - 1*100.4 = 9.16, and the wallet grew by exactly that.
			So(balances.RealizedPnL(), ShouldAlmostEqual, 9.16, 1e-9)
			So(balanceOf(balances, "EUR"), ShouldAlmostEqual, 200+9.16, 1e-9)
			So(balanceOf(balances, "ETH"), ShouldAlmostEqual, 0, 1e-12)
		})

		Convey("A losing round trip realizes a negative net and the basis is dropped when flat", func() {
			balances.ApplyFill("ETH/EUR", "buy", 1, 100, 0.4, "exec-1")
			balances.ApplyFill("ETH/EUR", "sell", 1, 90, 0.36, "exec-2")

			So(balances.RealizedPnL(), ShouldAlmostEqual, (90-0.36)-100.4, 1e-9) // -10.76
			_, hasBasis := balances.costBasis["ETH"]
			So(hasBasis, ShouldBeFalse)
		})

		Convey("Two buy lots blend into a fee-inclusive average cost basis", func() {
			balances.ApplyFill("ETH/EUR", "buy", 1, 100, 0, "exec-1") // basis 100
			balances.ApplyFill("ETH/EUR", "buy", 1, 50, 0, "exec-2")  // blended (100+50)/2 = 75

			blended, _ := balances.costBasis["ETH"].Float64()
			So(blended, ShouldAlmostEqual, 75, 1e-9)
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

func drainWallet(sub *qpool.BroadcastConsumer) map[string]any {
	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for {
		msg, err := sub.Wait(waitCtx)
		if err != nil {
			return nil
		}

		frame, _ := msg.Value.(map[string]any)

		if frame["event"] == "wallet" {
			return frame
		}
	}
}

func drainRawBalances(sub *qpool.BroadcastConsumer) map[string]any {
	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	msg, err := sub.Wait(waitCtx)
	if err != nil {
		return nil
	}

	frame, _ := msg.Value.(map[string]any)
	return frame
}
