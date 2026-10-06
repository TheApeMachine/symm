package data

import (
	"math"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMetricFinalize(t *testing.T) {
	Convey("Given a metric continuing a seeded Welford state", t, func() {
		metric := NewMetric("depth_zscore:bid", 1, UnitZScore, TimescaleRollingWindow)
		metric.center = 2
		metric.scale = 3

		Convey("a finite Raw updates center and scale", func() {
			So(metric.finalize(2), ShouldBeNil)
			So(metric.center, ShouldEqual, 1.5)
			So(metric.scale, ShouldBeGreaterThan, 0)
		})

		Convey("an undefined Raw fails before it can poison center or scale", func() {
			metric.Raw = math.NaN()

			err := metric.finalize(2)
			So(err, ShouldNotBeNil)
			So(strings.Contains(err.Error(), "raw is required"), ShouldBeTrue)
			So(metric.center, ShouldEqual, 2)
			So(metric.scale, ShouldEqual, 3)
		})

		Convey("an infinite Raw fails the same way", func() {
			metric.Raw = math.Inf(1)

			So(metric.finalize(2), ShouldNotBeNil)
			So(metric.center, ShouldEqual, 2)
			So(metric.scale, ShouldEqual, 3)
		})
	})
}
