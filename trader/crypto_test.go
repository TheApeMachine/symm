package trader

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestResendWallet(t *testing.T) {
	Convey("Given a crypto wallet cache", t, func() {
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
		}

		crypto.resendWallet()

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
