package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMetricKey(t *testing.T) {
	Convey("MetricKey", t, func() {
		Convey("It omits side when none", func() {
			So(MetricKey(MetricIgnition, SideNone), ShouldEqual, "ignition")
		})

		Convey("It suffixes side when set", func() {
			So(
				MetricKey(MetricArrivalRate, SideBuy),
				ShouldEqual,
				"arrival_rate:buy",
			)
		})
	})
}

func TestParseMetricKey(t *testing.T) {
	Convey("ParseMetricKey", t, func() {
		Convey("It splits metric:side", func() {
			metric, side := ParseMetricKey("arrival_rate:buy")
			So(metric, ShouldEqual, MetricArrivalRate)
			So(side, ShouldEqual, SideBuy)
		})

		Convey("It returns SideNone without colon", func() {
			metric, side := ParseMetricKey("ignition")
			So(metric, ShouldEqual, MetricIgnition)
			So(side, ShouldEqual, SideNone)
		})
	})
}

func TestMeasurementSample(t *testing.T) {
	Convey("Measurement.Sample", t, func() {
		measurement := &Measurement{
			Metrics: map[string]MetricSample{
				"ignition":           {Raw: 1.2},
				"arrival_rate:buy":   {Raw: 3.4},
				"arrival_rate:sell":  {Raw: 0.5},
			},
		}

		Convey("It finds undirected samples", func() {
			sample, ok := measurement.Sample(MetricIgnition, SideNone)
			So(ok, ShouldBeTrue)
			So(sample.Raw, ShouldEqual, 1.2)
		})

		Convey("It finds side-qualified samples", func() {
			sample, ok := measurement.Sample(MetricArrivalRate, SideBuy)
			So(ok, ShouldBeTrue)
			So(sample.Raw, ShouldEqual, 3.4)
		})

		Convey("It reports missing keys", func() {
			_, ok := measurement.Sample(MetricRVOL, SideNone)
			So(ok, ShouldBeFalse)
		})
	})
}

func BenchmarkMetricKey(b *testing.B) {
	for b.Loop() {
		_ = MetricKey(MetricArrivalRate, SideBuy)
	}
}
