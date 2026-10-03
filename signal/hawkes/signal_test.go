package hawkes_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/hawkes"
)

func TestHawkesTradeMetrics(t *testing.T) {
	Convey("Hawkes arrival-dynamics instrument computes complete honest metrics", t, func() {
		ctx := context.Background()
		arena := data.NewArenaOwner(4096)

		instrument := hawkes.NewSignal(ctx, arena)
		instrument.Transition(nmruntime.READY)

		now := time.Now()

		for step := 0; step < 20; step++ {
			prior := arena.NewMeasurement("ingress")
			prior.Label = "BTC/USD"
			prior.SeqIdx = int64(step + 1)
			prior.At = now.Add(time.Duration(step) * 50 * time.Millisecond)
			prior.From = prior.At

			prior.WriteMetric("price", 50000.0)
			prior.WriteMetric("qty", 1.0)
			prior.SetProvenance("channel", "trade")
			if step%2 == 0 {
				prior.SetProvenance("side", "buy")
			} else {
				prior.SetProvenance("side", "sell")
			}

			res := instrument.Step(prior)
			So(res, ShouldNotBeNil)

			_, hasCount := res.LookupMetric("event_count")
			So(hasCount, ShouldBeTrue)

			if step > 0 {
				_, hasRate := res.LookupMetric("arrival_rate")
				So(hasRate, ShouldBeTrue)
			}

			So(res.Maturity, ShouldBeGreaterThanOrEqualTo, 0)
			So(res.Maturity, ShouldBeLessThanOrEqualTo, 1.0)
		}
	})
}
