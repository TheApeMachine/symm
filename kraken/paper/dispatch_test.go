package paper

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestWebSocketMarksDeskReadyAtBoot(t *testing.T) {
	t.Cleanup(trading.ResetDeskReady)

	Convey("Given a paper websocket constructed before engine start", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		viper.Set("trading.paper.wallet_eur", 200.0)
		t.Cleanup(viper.Reset)

		So(trading.DeskReady(), ShouldBeFalse)

		_ = NewWebSocket(ctx, pool)

		Convey("It should process the balance subscribe before measurements arrive", func() {
			select {
			case <-time.After(200 * time.Millisecond):
				So(trading.DeskReady(), ShouldBeTrue)
			}
		})
	})
}
