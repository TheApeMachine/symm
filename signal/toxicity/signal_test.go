package toxicity_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
TestCalculate proves a real touch cancellation is separated from simultaneous
execution and emitted as retreat through the complete production boot graph.
*/
func TestCalculate(t *testing.T) {
	metrics := []types.MetricType{
		types.MetricTradeVolume,
		types.MetricFillVolume,
		types.MetricBestPrice,
		types.MetricTouchQuantity,
		types.MetricCancelledQuantity,
		types.MetricRetreatingQuantity,
	}

	Convey("Given a production-booted sincere Level3 touch", t, func() {
		market := tests.NewMarket(t.Context(), 3)
		So(market.Bootstrap(), ShouldBeNil)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() { wired.Close(); market.Close() })
		So(market.Warmup(wired.Crypto.Tick), ShouldBeNil)
		subject := market.Symbols[0]

		Convey("When its bid touch retreats beside a real ask execution", func() {
			So(market.Transition(
				tests.MarketStateLiquidityRetreat,
				wired.Crypto.Tick,
				subject,
			), ShouldBeNil)
			values := utils.LatestMeasurements(
				wired.Crypto.Thesis().Measurements,
				types.SourceToxicity,
				metrics,
			)

			for _, metric := range metrics {
				So(values[metric], ShouldContainKey, subject)
				So(values[metric][subject], ShouldBeGreaterThan, 0)
			}
		})
	})
}
