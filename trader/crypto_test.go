package trader

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
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
