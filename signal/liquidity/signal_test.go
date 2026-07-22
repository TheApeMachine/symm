package liquidity_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
TestCalculate proves an isolated thin touch becomes cross-sectional scarcity
through the complete production boot graph.
*/
func TestCalculate(t *testing.T) {
	metrics := []types.MetricType{
		types.MetricExecutableTouchDepth,
		types.MetricRelativeTouchDepth,
		types.MetricScarcityScore,
		types.MetricExecutableTouchDepthMedian,
		types.MetricReportedVolumeNotional,
		types.MetricReportedVolumeNotionalMedian,
	}

	Convey("Given a production-booted simulated cohort", t, func() {
		market := tests.NewMarket(t.Context(), 3)
		So(market.Bootstrap(), ShouldBeNil)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() { wired.Close(); market.Close() })
		So(market.Warmup(wired.Crypto.Tick), ShouldBeNil)
		subject := market.Symbols[0]

		Convey("When the subject touch becomes thin", func() {
			So(market.Transition(
				tests.MarketStateThinLiquidity,
				wired.Crypto.Tick,
				subject,
			), ShouldBeNil)
			values := utils.LatestMeasurements(
				wired.Crypto.Thesis().Measurements,
				types.SourceLiquidity,
				metrics,
			)

			for _, metric := range metrics {
				So(values[metric], ShouldContainKey, subject)
			}

			So(values[types.MetricExecutableTouchDepth][subject], ShouldBeGreaterThan, 0)
			So(values[types.MetricRelativeTouchDepth][subject], ShouldBeLessThan, 1)
			So(values[types.MetricScarcityScore][subject], ShouldBeGreaterThan, 0)
			So(values[types.MetricExecutableTouchDepthMedian][subject], ShouldBeGreaterThan,
				values[types.MetricExecutableTouchDepth][subject])
			So(values[types.MetricReportedVolumeNotional][subject], ShouldBeGreaterThan, 0)
			So(values[types.MetricReportedVolumeNotionalMedian][subject], ShouldBeGreaterThan, 0)
		})
	})
}
