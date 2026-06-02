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
)

func TestEnsureBalanceSnapshot(t *testing.T) {
	Convey("Given paper trading config", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		viper.Set("trading.model", "paper")
		viper.Set("trading.paper.wallet_eur", 200.0)
		viper.Set("market.quote_currency", "EUR")
		defer viper.Reset()

		_ = paper.NewWebSocket(ctx, pool)

		crypto, err := NewCrypto(ctx, pool, focus.NewSet())
		So(err, ShouldBeNil)

		subscriber := crypto.ui.Subscribe("test:crypto:balance", 4)

		go func() {
			_ = crypto.Tick()
		}()

		Convey("It should request and publish the seeded wallet", func() {
			select {
			case frame := <-subscriber.Incoming:
				payload, ok := frame.Value.(map[string]any)

				So(ok, ShouldBeTrue)
				So(payload["event"], ShouldEqual, "wallet")
				So(payload["Balance"], ShouldEqual, 200.0)
			case <-time.After(time.Second):
				So("wallet frame", ShouldBeBlank)
			}

			cancel()
			So(crypto.Close(), ShouldBeNil)
		})
	})
}
