package trading

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
)

func TestLedger_Register(t *testing.T) {
	Convey("Given a trading ledger", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 1, 4, nil)
		orders := pool.CreateBroadcastGroup("kraken:private", 10*time.Millisecond)
		ledger, err := NewLedger(ctx, pool, orders)

		So(err, ShouldBeNil)

		go func() {
			_ = ledger.Run()
		}()

		defer ledger.Close()

		viper.Set("trading.order_ack_timeout", time.Second)

		Convey("It should resolve add_order executions by cl_ord_id", func() {
			resultCh := ledger.Register("paper-cl-1")

			raw := pool.CreateBroadcastGroup("raw", 10*time.Millisecond)
			raw.Send(&qpool.QValue[any]{
				Type: public.ExecutionsChannel,
				Value: public.SocketMessage{
					Channel: public.ExecutionsChannel,
					Type:    "update",
					Data: []byte(
						`[{"cl_ord_id":"paper-cl-1","order_id":"O1","exec_type":"trade","order_status":"filled"}]`,
					),
				},
			})

			select {
			case result := <-resultCh:
				So(result.Success, ShouldBeTrue)
				So(result.OrderID, ShouldEqual, "O1")
			case <-time.After(2 * time.Second):
				So("timed out waiting for order ack", ShouldBeBlank)
			}
		})

		Convey("It should trip the breaker on ack timeout", func() {
			viper.Set("trading.order_ack_timeout", 20*time.Millisecond)
			_ = ledger.Register("paper-cl-timeout")

			time.Sleep(50 * time.Millisecond)

			So(ledger.Halted(), ShouldBeTrue)
		})
	})
}

func BenchmarkLedger_Register(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 1, 4, nil)
	orders := pool.CreateBroadcastGroup("kraken:private", 10*time.Millisecond)
	ledger, _ := NewLedger(ctx, pool, orders)

	go func() {
		_ = ledger.Run()
	}()

	defer ledger.Close()

	for b.Loop() {
		_ = ledger.Register("bench-cl")
	}
}
