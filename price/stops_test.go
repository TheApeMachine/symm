package price

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/market"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPredictionRegisterStop(t *testing.T) {
	Convey("Given an armed paper stop", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		t.Cleanup(func() { pool.Close() })

		prediction := NewPrediction(ctx, pool)
		t.Cleanup(func() { _ = prediction.Close() })

		exits := pool.CreateBroadcastGroup("exits", 10*time.Millisecond)
		subscriber := exits.Subscribe("test:stops", 8)
		prediction.RegisterStop("BTC/EUR", 99)

		go func() {
			_ = prediction.Tick()
		}()

		pool.CreateBroadcastGroup("tick", 10*time.Millisecond).Send(&qpool.QValue[any]{
			Value: market.TickerRow{Symbol: "BTC/EUR", Last: 98.5},
		})

		Convey("It should emit one stop hit at the trigger price", func() {
			select {
			case value := <-subscriber.Incoming:
				exitSignal, ok := value.Value.(engine.Exit)

				So(ok, ShouldBeTrue)
				So(exitSignal.Symbol, ShouldEqual, "BTC/EUR")
				So(exitSignal.Reason, ShouldEqual, engine.ExitReasonStopHit)
				So(exitSignal.LimitPrice, ShouldEqual, 99)
			case <-time.After(time.Second):
				t.Fatal("expected stop hit exit")
			}

			pool.CreateBroadcastGroup("tick", 10*time.Millisecond).Send(&qpool.QValue[any]{
				Value: market.TickerRow{Symbol: "BTC/EUR", Last: 98},
			})

			select {
			case value := <-subscriber.Incoming:
				t.Fatalf("expected stop to fire once, got %v", value.Value)
			case <-time.After(50 * time.Millisecond):
			}
		})
	})
}
