package private

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/kraken/public"
)

func TestNewWebSocketPaperMode(t *testing.T) {
	testconfig.Load(t)
	viper.Set("trading.model", "paper")
	viper.Set("trading.paper.default_one_way_latency", time.Nanosecond)

	Convey("Given paper trading model", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 4, nil)
		defer pool.Close()

		client := NewWebSocket(ctx, pool, "", "")

		Convey("It should delegate to the paper websocket", func() {
			So(client, ShouldNotBeNil)
			So(client.Connect(public.WebSocketAuthURL, "executions", 0), ShouldBeNil)
			So(client.Close(), ShouldBeNil)
		})
	})
}
