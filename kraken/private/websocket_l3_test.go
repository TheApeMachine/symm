package private

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/bus"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/kraken/public"
)

func TestNewWebSocketHybridMode(t *testing.T) {
	testconfig.Load(t)

	Convey("Given paper trading with L3 enabled", t, func() {
		viper.Set("trading.model", "paper")
		viper.Set("market.l3_enabled", true)
		defer viper.Set("market.l3_enabled", false)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		defer pool.Close()

		Convey("It should return hybrid without credentials but keep paper execution", func() {
			client := NewWebSocket(ctx, pool, "", "")

			So(client, ShouldNotBeNil)
			_, hybrid := client.(*hybridWebSocket)
			So(hybrid, ShouldBeFalse)
		})

		Convey("It should build a data-only socket with credentials", func() {
			dataOnly, err := newDataOnlyWebSocket(ctx, pool, "key", "c2VjcmV0")

			So(err, ShouldBeNil)
			So(dataOnly, ShouldNotBeNil)
			So(dataOnly.dataOnly, ShouldBeTrue)
			So(dataOnly.subscriber, ShouldBeNil)
			So(dataOnly.level3, ShouldNotBeNil)
		})
	})
}

func TestWebSocketPublishLevel3DataOnly(t *testing.T) {
	Convey("Given a data-only websocket", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 1, 4, nil)
		defer pool.Close()

		level3 := bus.Group(pool, "level3", 0)
		subscriber := level3.Subscribe("test:level3", 4)

		websocketClient := &WebSocket{
			ctx:      ctx,
			raw:      bus.Group(pool, "raw", 0),
			level3:   level3,
			dataOnly: true,
		}

		websocketClient.publishRaw(authFrame{
			Channel: public.Level3Channel,
			Type:    "update",
			Data:    []byte(`[{"symbol":"BTC/EUR"}]`),
		})

		Convey("It should mirror level3 frames onto the level3 bus", func() {
			message := <-subscriber.Incoming
			envelope, ok := message.Value.(map[string]any)

			So(ok, ShouldBeTrue)
			So(envelope["channel"], ShouldEqual, public.Level3Channel)
		})
	})
}
