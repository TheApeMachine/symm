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

func TestDerivativesTradeMetrics(t *testing.T) {
	Convey("Derivatives trade instrument publishes complete honest liquidation metrics", t, func() {
		ctx := context.Background()
		arena := data.NewArenaOwner(4096)

		instrument := derivatives.NewTrade(ctx, arena)
		instrument.Transition(nmruntime.READY)

		now := time.Now()

		for step := 0; step < 10; step++ {
			prior := arena.NewMeasurement("ingress")
			prior.Label = "BTC/USD:perp"
			prior.SeqIdx = int64(step + 1)
			prior.At = now.Add(time.Duration(step) * 100 * time.Millisecond)
			prior.From = prior.At

			prior.WriteMetric("price", 50000.0)
			prior.WriteMetric("qty", 1.5)
			prior.SetProvenance("side", "buy")
			if step%2 == 1 {
				prior.SetProvenance("type", "liquidation")
			}

			res := instrument.Step(prior)
			So(res, ShouldNotBeNil)

			_, hasGrossTrade := res.LookupMetric("gross_derivative_trade_notional")
			So(hasGrossTrade, ShouldBeTrue)

			_, hasGrossLiq := res.LookupMetric("gross_liquidation_notional")
			So(hasGrossLiq, ShouldBeTrue)

			_, hasNetLiq := res.LookupMetric("net_liquidation_notional")
			So(hasNetLiq, ShouldBeTrue)

			if step > 2 {
				So(res.Maturity, ShouldBeGreaterThan, 0)
			}
		}
	})
}
