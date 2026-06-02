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

func TestWebSocketOrderAckRejectsUnknownFrame(t *testing.T) {
	t.Cleanup(viper.Reset)

	Convey("Given a market order with no reference price", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		_ = NewWebSocket(ctx, pool)

		viper.Set("trading.order_ack_timeout", time.Second)

		client, clientErr := trading.NewOrder(ctx, pool)

		So(clientErr, ShouldBeNil)

		defer client.Close()

		resultCh, addErr := client.AddOrder(trading.AddParams{
			OrderType: trading.Market,
			Side:      trading.Buy,
			Symbol:    "BTC/EUR",
			OrderQty:  0.001,
			ClOrdID:   "paper-ws-reject-1",
		})

		So(addErr, ShouldBeNil)

		defer client.ReleaseOrderResult(resultCh)

		select {
		case result := <-resultCh:
			So(result.Success, ShouldBeFalse)
			So(result.ClOrdID, ShouldEqual, "paper-ws-reject-1")
		case <-time.After(2 * time.Second):
			So("timed out waiting for paper rejection ack", ShouldBeBlank)
		}
	})
}
