package data

import (
	"math"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

func TestMetricSetSNM(t *testing.T) {
	Convey("Given a Metric with a raw observation", t, func() {
		metric := NewMetric("price", 110.0, UnitPrice, TimescaleInstantaneous)

		Convey("When setSNM is called with a prior value", func() {
			metric.setSNM(100.0)

			// delta = 110 - 100 = 10
			// denom = |100| + |10| = 110
			// snm = 10 / 110 = 0.09090909090909091
			expected := 10.0 / 110.0
			So(metric.SNM(), ShouldAlmostEqual, expected, 0.0001)
		})

		Convey("When setSNM is called with a higher prior (negative movement)", func() {
			metric.setSNM(120.0)

			// delta = 110 - 120 = -10
			// denom = |120| + |-10| = 130
			// snm = -10 / 130 = -0.07692307692307693
			expected := -10.0 / 130.0
			So(metric.SNM(), ShouldAlmostEqual, expected, 0.0001)
		})

		Convey("When setSNM is called with identical prior (zero movement)", func() {
			metric.setSNM(110.0)
			So(metric.SNM(), ShouldEqual, 0.0)
		})

		Convey("When both raw and prior are zero", func() {
			zeroMetric := NewMetric("zero", 0.0, UnitDimensionless, TimescaleTick)
			zeroMetric.setSNM(0.0)
			So(zeroMetric.SNM(), ShouldEqual, 0.0)
		})
	})
}

func TestMetricFinalize(t *testing.T) {
	Convey("Given a Metric", t, func() {
		Convey("When SNM is within [-1, 1], validation passes", func() {
			metric := NewMetric("spread", 0.5, UnitSpread, TimescaleInstantaneous)
			metric.setSNM(0.4)
			So(metric.finalize(), ShouldBeNil)
		})

		Convey("When SNM is NaN, validation returns an error", func() {
			metric := NewMetric("spread", 0.5, UnitSpread, TimescaleInstantaneous)
			metric.snm = math.NaN()
			So(metric.finalize(), ShouldNotBeNil)
		})

		Convey("When SNM is out of bounds (> 1.0), validation returns an error", func() {
			metric := NewMetric("spread", 0.5, UnitSpread, TimescaleInstantaneous)
			metric.snm = 1.5
			So(metric.finalize(), ShouldNotBeNil)
		})
	})
}

func TestNewExactMetric(t *testing.T) {
	Convey("Given NewExactMetric with a decimal", t, func() {
		exactDecimal := decimal.NewFromFloat64(60000.50)
		metric := NewExactMetric("price", exactDecimal, UnitPrice, TimescaleInstantaneous)

		So(metric.Exact(), ShouldEqual, exactDecimal)
		So(metric.Raw(), ShouldAlmostEqual, 60000.50, 0.001)

		Convey("Can set SNM internally on exact metric", func() {
			metric.setSNM(60000.0)
			So(metric.SNM(), ShouldBeGreaterThan, 0.0)
		})
	})
}
