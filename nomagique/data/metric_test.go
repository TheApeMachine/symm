package data

import (
	"math"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMetricFinalize(t *testing.T) {
	Convey("Given a metric whose stream has seen 2 and 2+3*sqrt(2)", t, func() {
		metric := NewMetric("depth_zscore:bid", 1, UnitZScore, TimescaleRollingWindow)
		state := &standardizer{}
		state.step(2)
		state.step(2 + 3*math.Sqrt(2))
		priorMean := state.mean

		Convey("a finite Raw is standardized against the prior moments, then folded in", func() {
			So(metric.finalize(state), ShouldBeNil)
			So(metric.center, ShouldAlmostEqual, priorMean, 1e-12)
			So(metric.scale, ShouldAlmostEqual, 3, 1e-12)
			So(metric.Standardizable(), ShouldBeTrue)
			So(metric.Standardized, ShouldAlmostEqual, (1-priorMean)/3, 1e-12)
			So(metric.Normalized, ShouldAlmostEqual, math.Tanh((1-priorMean)/3), 1e-12)
			So(state.count, ShouldEqual, 3)
		})

		Convey("an undefined Raw fails before it can poison the stream", func() {
			metric.Raw = math.NaN()

			err := metric.finalize(state)
			So(err, ShouldNotBeNil)
			So(strings.Contains(err.Error(), "raw is required"), ShouldBeTrue)
			So(state.count, ShouldEqual, 2)
			So(state.mean, ShouldEqual, priorMean)
		})

		Convey("an infinite Raw fails the same way", func() {
			metric.Raw = math.Inf(1)

			So(metric.finalize(state), ShouldNotBeNil)
			So(state.count, ShouldEqual, 2)
			So(state.mean, ShouldEqual, priorMean)
		})
	})

	Convey("Given a metric whose stream has a single prior observation", t, func() {
		metric := NewMetric("depth_zscore:bid", 5, UnitZScore, TimescaleRollingWindow)
		state := &standardizer{}
		state.step(1)

		Convey("its z-score is undefined rather than zero evidence", func() {
			So(metric.finalize(state), ShouldBeNil)
			So(metric.Standardizable(), ShouldBeFalse)
			So(metric.Standardized, ShouldEqual, 0)
			So(metric.Normalized, ShouldEqual, 0)
		})
	})
}
