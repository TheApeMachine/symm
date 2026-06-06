package toxicity

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/bus"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
)

func TestToxicityHandleLevel3(t *testing.T) {
	Convey("Given a level3 update on the bus", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		tox := NewToxicity(ctx, pool)
		tox.l3Active = true
		level3 := bus.Group(pool, "level3", 10*time.Millisecond)
		tox.subscribers["level3"] = level3.Subscribe("toxicity:test-level3", 16)

		now := time.Now()
		payload := []byte(`[{
			"symbol":"BTC/EUR",
			"bids":[
				{"event":"add","order_id":"l3-2","limit_price":100,"order_qty":15,"timestamp":"` + now.Format(time.RFC3339Nano) + `"},
				{"event":"delete","order_id":"l3-2","limit_price":100,"order_qty":15,"timestamp":"` + now.Format(time.RFC3339Nano) + `"}
			],
			"asks":[]
		}]`)

		level3.Send(&qpool.QValue[any]{Value: map[string]any{
			"channel": public.Level3Channel,
			"type":    "update",
			"data":    payload,
		}})

		tox.tracker.ObserveMid("BTC/EUR", market.Pair{}, 100)

		err := tox.handleLevel3(<-tox.subscribers["level3"].Incoming)

		Convey("It should classify per-order churn as toxic", func() {
			So(err, ShouldBeNil)
			So(tox.tracker.IsToxic("BTC/EUR", 100, now), ShouldBeTrue)
		})
	})

	Convey("Given a level3 envelope without order data", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		tox := NewToxicity(ctx, pool)
		level3 := bus.Group(pool, "level3", 10*time.Millisecond)
		tox.subscribers["level3"] = level3.Subscribe("toxicity:test-level3-empty", 16)

		level3.Send(&qpool.QValue[any]{Value: map[string]any{
			"channel": public.Level3Channel,
			"type":    "update",
		}})

		err := tox.handleLevel3(<-tox.subscribers["level3"].Incoming)

		Convey("It should ignore the frame without error", func() {
			So(err, ShouldBeNil)
		})
	})
}
