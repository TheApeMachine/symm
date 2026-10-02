package derivatives_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/derivatives"
)

func TestDerivativesTickerMetrics(t *testing.T) {
	Convey("Derivatives ticker instrument publishes complete honest metrics", t, func() {
		ctx := context.Background()
		arena := data.NewArenaOwner(4096)

		instrument := derivatives.NewTicker(ctx, arena)
		instrument.Transition(nmruntime.READY)

		now := time.Now()

		for step := 0; step < 10; step++ {
			prior := arena.NewMeasurement("ingress")
			prior.Label = "BTC/USD:perp"
			prior.SeqIdx = int64(step + 1)
			prior.At = now.Add(time.Duration(step) * 100 * time.Millisecond)
			prior.From = prior.At

			prior.WriteMetric("last", 50000.0+float64(step)*10.0)
			prior.WriteMetric("index_price", 49990.0+float64(step)*8.0)
			prior.WriteMetric("mark_price", 49995.0+float64(step)*9.0)
			prior.WriteMetric("open_interest", 1000.0+float64(step)*50.0)

			res := instrument.Step(prior)
			So(res, ShouldNotBeNil)

			_, hasDerivPrice := res.LookupMetric("derivative_price")
			So(hasDerivPrice, ShouldBeTrue)

			_, hasRefPrice := res.LookupMetric("reference_price")
			So(hasRefPrice, ShouldBeTrue)

			_, hasBasis := res.LookupMetric("basis")
			So(hasBasis, ShouldBeTrue)

			_, hasLogBasis := res.LookupMetric("log_basis")
			So(hasLogBasis, ShouldBeTrue)

			if step > 0 {
				_, hasOIChange := res.LookupMetric("open_interest_change")
				So(hasOIChange, ShouldBeTrue)

				_, hasOIGrowthRate := res.LookupMetric("open_interest_growth_rate")
				So(hasOIGrowthRate, ShouldBeTrue)

				_, hasBasisRate := res.LookupMetric("basis_rate")
				So(hasBasisRate, ShouldBeTrue)

				_, hasReturnGap := res.LookupMetric("return_gap")
				So(hasReturnGap, ShouldBeTrue)
			}

			if step > 2 {
				So(res.Maturity, ShouldBeGreaterThan, 0)
			}
		}
	})
}
