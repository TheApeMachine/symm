package private

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/kraken/paper"
	"github.com/theapemachine/symm/kraken/public"
)

func TestExecutionSystems(t *testing.T) {
	testconfig.Load(t)

	Convey("Given paper trading with L3 enabled", t, func() {
		viper.Set("trading.model", "paper")
		viper.Set("market.l3_enabled", true)
		defer viper.Set("market.l3_enabled", false)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 4, nil)
		defer pool.Close()

		quotes := broker.EnsureQuoteCache(ctx, pool)

		Convey("It should register paper only without credentials", func() {
			runtimes := ExecutionSystems(ctx, pool, "", "", quotes)

			So(len(runtimes), ShouldEqual, 1)
			_, paperRuntime := runtimes[0].(*paper.WebSocket)
			So(paperRuntime, ShouldNotBeNil)
		})

		Convey("It should register paper and L3 as separate runtimes with credentials", func() {
			runtimes := ExecutionSystems(ctx, pool, "key", "c2VjcmV0", quotes)

			So(len(runtimes), ShouldEqual, 2)
			_, paperRuntime := runtimes[0].(*paper.WebSocket)
			So(paperRuntime, ShouldNotBeNil)

			level3, ok := runtimes[1].(*WebSocket)
			So(ok, ShouldBeTrue)
			So(level3.dataOnly, ShouldBeTrue)
		})
	})
}

func TestNewLevel3WebSocket(t *testing.T) {
	Convey("Given API credentials", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 4, nil)
		defer pool.Close()

		Convey("It should build a data-only socket", func() {
			level3, err := NewLevel3WebSocket(ctx, pool, "key", "c2VjcmV0")

			So(err, ShouldBeNil)
			So(level3, ShouldNotBeNil)
			So(level3.dataOnly, ShouldBeTrue)
			So(level3.subscriber, ShouldBeNil)
			So(level3.level3, ShouldNotBeNil)
		})
	})
}

func TestWebSocketPublishLevel3DataOnly(t *testing.T) {
	Convey("Given a data-only websocket", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 4, nil)
		defer pool.Close()

		level3Bus := pool.CreateBroadcastGroup("level3", 0)
		subscriber := level3Bus.Subscribe("test:level3", 4)

		websocketClient := &WebSocket{
			ctx:      ctx,
			raw:      pool.CreateBroadcastGroup("raw", 0),
			level3:   level3Bus,
			dataOnly: true,
		}

		websocketClient.publishRaw(authFrame{
			Channel: public.Level3Channel,
			Type:    "update",
			Data:    []byte(`[{"symbol":"BTC/EUR"}]`),
		})

		Convey("It should mirror level3 frames onto the level3 bus", func() {
			waitCtx, waitCancel := context.WithTimeout(ctx, 2*time.Second)
			defer waitCancel()

			message, err := subscriber.Wait(waitCtx)
			So(err, ShouldBeNil)

			envelope, ok := message.Value.(map[string]any)

			So(ok, ShouldBeTrue)
			So(envelope["channel"], ShouldEqual, public.Level3Channel)
		})
	})
}
