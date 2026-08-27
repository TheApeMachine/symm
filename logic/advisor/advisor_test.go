package advisor

import (
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

/*
testMeasurement builds one projected measurement with the given metric values.
*/
func testMeasurement(symbol, source string, at time.Time, values map[string]float64) *data.Measurement[float64] {
	metrics := make(map[string]data.Metric[float64], len(values))

	for name, value := range values {
		metrics[name] = data.Metric[float64]{Raw: value}
	}

	return &data.Measurement[float64]{
		Label:   symbol,
		Source:  source,
		At:      at,
		Metrics: metrics,
	}
}

func TestAdvisorStep(t *testing.T) {
	Convey("Given a composed-metric Advisor", t, func() {
		advisor := NewAdvisor(
			"advisor:liquidity",
			types.KindLiquidity,
			MetricBinding{Source: "liquidity", Metric: "relative_spread"},
		)

		Convey("a nil or errored or unlabeled measurement produces no perspective", func() {
			So(advisor.Step(nil), ShouldBeNil)
			So(advisor.Step(&data.Measurement[float64]{Err: errors.New("bad")}), ShouldBeNil)
			So(advisor.Step(&data.Measurement[float64]{Label: "", Source: "liquidity"}), ShouldBeNil)
		})

		Convey("a measurement from an uncomposed source produces no reading", func() {
			perspective := advisor.Step(testMeasurement("TEST/USD", "cvd", time.Unix(0, 0), map[string]float64{
				"relative_spread": 0.01,
			}))

			So(perspective, ShouldNotBeNil)
			So(perspective.Count, ShouldEqual, 0)
		})

		Convey("the first observation is not ready: no baseline departure yet", func() {
			perspective := advisor.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(0, 0), map[string]float64{
				"relative_spread": 0.01,
			}))

			So(perspective, ShouldNotBeNil)
			So(perspective.Kind, ShouldEqual, types.KindLiquidity)
			So(perspective.Count, ShouldEqual, 1)
			So(perspective.Readings[0].Ready, ShouldBeFalse)
		})

		Convey("a second observation makes the reading ready with derived context", func() {
			advisor.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(0, 0), map[string]float64{
				"relative_spread": 0.01,
			}))
			perspective := advisor.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(0, int64(time.Second)), map[string]float64{
				"relative_spread": 0.02,
			}))

			So(perspective, ShouldNotBeNil)
			So(perspective.Readings[0].Ready, ShouldBeTrue)
			So(perspective.Readings[0].Value, ShouldEqual, 0.02)
			So(perspective.Readings[0].Velocity, ShouldEqual, 0.01)
		})

		Convey("two advisors fed identical measurements emit identical readings", func() {
			left := NewAdvisor("a", types.KindLiquidity, MetricBinding{Source: "liquidity", Metric: "relative_spread"})
			right := NewAdvisor("b", types.KindLiquidity, MetricBinding{Source: "liquidity", Metric: "relative_spread"})

			feed := func(advisor *Advisor) *types.Perspective {
				advisor.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(0, 0), map[string]float64{"relative_spread": 0.01}))
				return advisor.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(0, int64(time.Second)), map[string]float64{"relative_spread": 0.02}))
			}

			leftPerspective := feed(left)
			rightPerspective := feed(right)

			So(leftPerspective.Readings[0], ShouldResemble, rightPerspective.Readings[0])
			So(leftPerspective.Count, ShouldEqual, rightPerspective.Count)
		})

		Convey("each composed metric occupies its own independent stream", func() {
			multi := NewAdvisor(
				"advisor:multi",
				types.KindState,
				MetricBinding{Source: "liquidity", Metric: "relative_spread"},
				MetricBinding{Source: "depthflow", Metric: "book_imbalance"},
			)

			multi.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(0, 0), map[string]float64{
				"relative_spread": 0.01,
			}))
			multi.Step(testMeasurement("TEST/USD", "depthflow", time.Unix(0, 0), map[string]float64{
				"book_imbalance": 0.5,
			}))

			perspective := multi.Step(testMeasurement("TEST/USD", "liquidity", time.Unix(0, int64(time.Second)), map[string]float64{
				"relative_spread": 0.02,
			}))

			So(perspective, ShouldNotBeNil)
			So(perspective.Count, ShouldEqual, 2)

			// The liquidity metric advanced to two observations and is ready;
			// the depthflow metric has one and is not.
			ready := 0

			for index := 0; index < perspective.Count; index++ {
				if perspective.Readings[index].Ready {
					ready++
				}
			}

			So(ready, ShouldEqual, 1)
		})
	})
}

/*
BenchmarkStep measures the steady-state cost of one perspective step after the
pipeline is warm. The only allocation is the emitted Perspective value.
*/
func BenchmarkStep(b *testing.B) {
	advisor := NewAdvisor("advisor:liquidity", types.KindLiquidity, MetricBinding{Source: "liquidity", Metric: "relative_spread"})
	at := time.Unix(0, 0)
	measurement := testMeasurement("TEST/USD", "liquidity", at, map[string]float64{"relative_spread": 0.01})
	advisor.Step(measurement)
	measurement.At = time.Unix(0, int64(time.Second))

	b.ReportAllocs()
	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		_ = advisor.Step(measurement)
	}
}
