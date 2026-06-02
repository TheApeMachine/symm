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
	t.Cleanup(viper.Reset)

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

			PublishLedgerAck(pool, public.SocketMessage{
				Channel: public.ExecutionsChannel,
				Type:    "update",
				Data: []byte(
					`[{"cl_ord_id":"paper-cl-1","order_id":"O1","exec_type":"trade","order_status":"filled"}]`,
				),
			})

			select {
			case result := <-resultCh:
				So(result.Success, ShouldBeTrue)
				So(result.OrderID, ShouldEqual, "O1")
				ledger.ReleaseResult(resultCh)
			case <-time.After(2 * time.Second):
				So("timed out waiting for order ack", ShouldBeBlank)
			}
		})
	})
}

func TestLedger_AckSurvivesRawFlood(t *testing.T) {
	t.Cleanup(viper.Reset)

	Convey("Given a saturated raw bus", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 1, 4, nil)
		orders := pool.CreateBroadcastGroup("kraken:private", 10*time.Millisecond)
		ledger, err := NewLedger(ctx, pool, orders)

		So(err, ShouldBeNil)

		go func() {
			_ = ledger.Run()
		}()

		defer ledger.Close()

		raw := pool.CreateBroadcastGroup("raw", 10*time.Millisecond)
		flood := raw.Subscribe("test:ledger:flood", 1)

		for range 512 {
			raw.Send(&qpool.QValue[any]{
				Type:  public.TradesChannel,
				Value: public.SocketMessage{Channel: public.TradesChannel},
			})
		}

		select {
		case <-flood.Incoming:
		default:
		}

		viper.Set("trading.order_ack_timeout", time.Second)
		resultCh := ledger.Register("paper-cl-flood")

		PublishLedgerAck(pool, public.SocketMessage{
			Channel: public.ExecutionsChannel,
			Type:    "update",
			Data: []byte(
				`[{"cl_ord_id":"paper-cl-flood","order_id":"O2","exec_type":"trade","order_status":"filled"}]`,
			),
		})

		select {
		case result := <-resultCh:
			So(result.Success, ShouldBeTrue)
			So(result.OrderID, ShouldEqual, "O2")
			ledger.ReleaseResult(resultCh)
		case <-time.After(2 * time.Second):
			So("timed out waiting for ledger ack under raw flood", ShouldBeBlank)
		}
	})
}

func TestLedger_AckTimeout(t *testing.T) {
	t.Cleanup(viper.Reset)

	Convey("Given a fresh trading ledger", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 1, 4, nil)
		orders := pool.CreateBroadcastGroup("kraken:private", 10*time.Millisecond)
		ledger, err := NewLedger(ctx, pool, orders)

		So(err, ShouldBeNil)

		go func() {
			_ = ledger.Run()
		}()

		defer ledger.Close()

		viper.Set("trading.model", "paper")
		viper.Set("trading.order_ack_timeout", 20*time.Millisecond)
		resultCh := ledger.Register("paper-cl-timeout-2")

		time.Sleep(50 * time.Millisecond)

		So(ledger.Halted(), ShouldBeFalse)

		select {
		case result := <-resultCh:
			So(result.Success, ShouldBeFalse)
			So(result.Error, ShouldEqual, "order ack timeout")
			ledger.ReleaseResult(resultCh)
		case <-time.After(time.Second):
			So("timed out waiting for breaker result", ShouldBeBlank)
		}
	})
}

func TestLedger_LiveRejectTrips(t *testing.T) {
	t.Cleanup(viper.Reset)

	Convey("Given a live trading ledger", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 1, 4, nil)
		orders := pool.CreateBroadcastGroup("kraken:private", 10*time.Millisecond)
		ledger, err := NewLedger(ctx, pool, orders)

		So(err, ShouldBeNil)

		go func() {
			_ = ledger.Run()
		}()

		defer ledger.Close()

		viper.Set("trading.model", "live")
		resultCh := ledger.Register("live-cl-reject-1")

		PublishLedgerAck(pool, public.SocketMessage{
			Channel: public.ExecutionsChannel,
			Type:    "update",
			Data: []byte(
				`[{"cl_ord_id":"live-cl-reject-1","exec_type":"rejected","order_status":"rejected"}]`,
			),
		})

		select {
		case result := <-resultCh:
			So(result.Success, ShouldBeFalse)
			ledger.ReleaseResult(resultCh)
		case <-time.After(time.Second):
			So("timed out waiting for rejection", ShouldBeBlank)
		}

		So(ledger.Halted(), ShouldBeTrue)
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
