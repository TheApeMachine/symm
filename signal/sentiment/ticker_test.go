package sentiment_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/sentiment"
)

func TestSentimentTickerMetrics(t *testing.T) {
	Convey("Sentiment ticker instrument measures cross-sectional breadth, returns, and dispersion", t, func() {
		ctx := context.Background()
		arena := data.NewArenaOwner(4096)

		instrument := sentiment.NewTicker(ctx, arena)
		instrument.Transition(nmruntime.READY)

		now := time.Now()
		symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD", "ADA/USD", "XRP/USD"}

		for step := 0; step < 10; step++ {
			for idx, symbol := range symbols {
				prior := arena.NewMeasurement("ingress")
				prior.Label = symbol
				prior.SeqIdx = int64(step*len(symbols) + idx + 1)
				prior.At = now.Add(time.Duration(step*len(symbols)+idx) * 100 * time.Millisecond)
				prior.From = prior.At

				// prices changing
				price := 100.0 * float64(idx+1) * (1.0 + float64(step)*0.01*float64(idx-2))
				prior.WriteMetric("last", price)

				res := instrument.Step(prior)
				So(res, ShouldNotBeNil)

				if step > 1 {
					_, hasMembers := res.LookupMetric("valid_member_count")
					So(hasMembers, ShouldBeTrue)

					_, hasBreadth := res.LookupMetric("breadth")
					So(hasBreadth, ShouldBeTrue)

					_, hasMedianReturn := res.LookupMetric("median_return")
					So(hasMedianReturn, ShouldBeTrue)

					_, hasReturnMad := res.LookupMetric("return_mad")
					So(hasReturnMad, ShouldBeTrue)
				}
			}
		}
	})
}
