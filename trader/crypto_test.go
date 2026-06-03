package trader

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/kraken/paper"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestResendWallet(t *testing.T) {
	Convey("Given a crypto wallet cache", t, func() {
		t.Cleanup(viper.Reset)
		viper.Set("market.quote_currency", "EUR")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		ui := pool.CreateBroadcastGroup("ui", 10*time.Millisecond)
		subscriber := ui.Subscribe("test:trader:ui", 4)

		crypto := &Crypto{
			ctx:       ctx,
			ui:        ui,
			cash:      198.5,
			inventory: map[string]float64{"BTC": 0.01},
			avgEntry:  map[string]float64{"BTC": 50_000},
			marks:     map[string]float64{"BTC/EUR": 51_000},
		}

		err := crypto.resendWallet()
		So(err, ShouldBeNil)

		Convey("It should publish the cached wallet snapshot to ui", func() {
			select {
			case frame := <-subscriber.Incoming:
				payload, ok := frame.Value.(map[string]any)

				So(ok, ShouldBeTrue)
				So(payload["event"], ShouldEqual, "wallet")
				So(payload["Balance"], ShouldEqual, 198.5)

				inventory, ok := payload["Inventory"].(map[string]float64)

				So(ok, ShouldBeTrue)
				So(inventory["BTC"], ShouldEqual, 0.01)

				avgEntry, ok := payload["AvgEntry"].(map[string]float64)

				So(ok, ShouldBeTrue)
				So(avgEntry["BTC"], ShouldEqual, 50_000)

				marks, ok := payload["Marks"].(map[string]float64)

				So(ok, ShouldBeTrue)
				So(marks["BTC/EUR"], ShouldEqual, 51_000)
			case <-time.After(500 * time.Millisecond):
				So("wallet frame", ShouldBeBlank)
			}
		})
	})
}

func TestCryptoResolveSellQuantity(t *testing.T) {
	Convey("Given inventory for the base asset", t, func() {
		crypto := &Crypto{
			inventory: map[string]float64{"BTC": 0.02},
		}

		action := crypto.resolveSellQuantity(perspectives.Action{
			Symbol: "BTC/EUR",
			Side:   trading.Sell,
		})

		Convey("It should size the exit from inventory", func() {
			So(action.Quantity, ShouldEqual, 0.02)
		})
	})
}

func TestCryptoReceivesPaperBalanceAfterSubscribe(t *testing.T) {
	t.Cleanup(trading.ResetDeskReady)

	Convey("Given paper boot before crypto subscribes to raw", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		viper.Set("trading.model", "paper")
		viper.Set("trading.paper.wallet_eur", 200.0)
		viper.Set("market.quote_currency", "EUR")
		t.Cleanup(viper.Reset)

		_ = paper.NewWebSocket(ctx, pool)

		crypto := NewCrypto(ctx, pool, focus.NewSet())
		subscriber := crypto.ui.Subscribe("test:trader:paper-wallet", 4)

		go func() {
			_ = crypto.Tick()
		}()

		Convey("It should request and publish the seeded paper wallet", func() {
			select {
			case frame := <-subscriber.Incoming:
				payload, ok := frame.Value.(map[string]any)

				So(ok, ShouldBeTrue)
				So(payload["event"], ShouldEqual, "wallet")
				So(payload["Balance"], ShouldEqual, 200.0)
			case <-time.After(time.Second):
				So("wallet frame", ShouldBeBlank)
			}
		})
	})
}

func TestCryptoHandleActionGuards(t *testing.T) {
	Convey("Given an open focus stream", t, func() {
		streams := focus.NewSet()
		streams.Add("BTC/EUR")

		crypto := &Crypto{
			streams: streams,
			desk:    nil,
		}

		viper.Set("trading.paper.wallet_eur", 200.0)
		defer viper.Set("trading.paper.wallet_eur", 0)

		entry := perspectives.ActionFromMeasurement(
			perspectives.ActionLimit,
			perspectives.Measurement{Symbol: "BTC/EUR", Last: 50_000},
		)

		Convey("When an entry action arrives while already holding", func() {
			crypto.handleAction(entry)

			Convey("It should not duplicate the open position", func() {
				So(streams.Has("BTC/EUR"), ShouldBeTrue)
			})
		})
	})
}
