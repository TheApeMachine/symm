package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestHollowPressure(t *testing.T) {
	Convey("Given toxicity evidence without a complete buy touch", t, func() {
		thesis := types.NewThesis(nil)
		thesis.Measurements.Store(types.SourceToxicity, []*types.Measurement{{
			Source: types.SourceToxicity,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricCancelledQuantity, types.SideBuy): {
					Raw: 4,
				},
			},
		}})

		Convey("It should report that pressure is not ready", func() {
			pressure, ready := hollowPressure(thesis, "BTC/USD")

			So(pressure, ShouldEqual, 0.0)
			So(ready, ShouldBeFalse)
		})
	})

	Convey("Given a cancellation and its remaining buy touch", t, func() {
		thesis := types.NewThesis(nil)
		thesis.Measurements.Store(types.SourceToxicity, []*types.Measurement{{
			Source: types.SourceToxicity,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricCancelledQuantity, types.SideBuy): {
					Raw: 4,
				},
				types.MetricKey(types.MetricTouchQuantity, types.SideBuy): {
					Raw: 6,
				},
			},
		}})

		Convey("It should normalize the removed quantity against the prior touch", func() {
			pressure, ready := hollowPressure(thesis, "BTC/USD")

			So(pressure, ShouldAlmostEqual, 0.4)
			So(ready, ShouldBeTrue)
		})
	})
}
