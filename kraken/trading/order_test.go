package trading

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
)

func TestNewOrderClient(t *testing.T) {
	t.Cleanup(viper.Reset)
	viper.Set("trading.order_ack_timeout", time.Second)

	Convey("Given an order client", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		client, err := NewOrder(ctx, pool)

		So(err, ShouldBeNil)

		Convey("It should start halted until orders flow", func() {
			So(client.Halted(), ShouldBeFalse)
		})
	})
}

func TestAddParamsJSONTags(t *testing.T) {
	Convey("Given order params", t, func() {
		params := AddParams{
			OrderType:  Limit,
			Side:       Buy,
			Symbol:     "BTC/EUR",
			OrderQty:   0.01,
			LimitPrice: 50_000,
			ClOrdID:    "test-order",
		}

		Convey("It should retain side and type constants", func() {
			So(string(params.OrderType), ShouldEqual, "limit")
			So(string(params.Side), ShouldEqual, "buy")
			So(params.ClOrdID, ShouldEqual, "test-order")
		})
	})
}

func BenchmarkNewOrder(b *testing.B) {
	ctx := context.Background()
	viper.Set("trading.order_ack_timeout", time.Second)

	for b.Loop() {
		pool := qpool.NewQ(ctx, 1, 4, nil)
		client, _ := NewOrder(ctx, pool)
		_ = client
		pool.Close()
	}
}
