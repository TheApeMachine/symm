package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestMetricKey proves directional metric keys stay stable for repeated callers
while ParseMetricKey and Sample retain the flat wire contract.
*/
func TestMetricKey(t *testing.T) {
	Convey("Given directional measurement keys", t, func() {
		buyKey := MetricKey(MetricArrivalRate, SideBuy)
		sellKey := MetricKey(MetricArrivalRate, SideSell)
		noneKey := MetricKey(MetricStrength, SideNone)

		Convey("They should preserve the flat metric wire shape", func() {
			So(buyKey, ShouldEqual, "arrival_rate:buy")
			So(sellKey, ShouldEqual, "arrival_rate:sell")
			So(noneKey, ShouldEqual, "strength")

			metric, side := ParseMetricKey(buyKey)
			So(metric, ShouldEqual, MetricArrivalRate)
			So(side, ShouldEqual, SideBuy)
		})

		Convey("Sample should read back the same keyed values", func() {
			measurement := &Measurement{
				Metrics: map[string]MetricSample{
					buyKey:  {Raw: 1.5},
					sellKey: {Raw: 0.7},
				},
			}

			buy, ok := measurement.Sample(MetricArrivalRate, SideBuy)
			So(ok, ShouldBeTrue)
			So(buy.Raw, ShouldEqual, 1.5)
		})
	})
}

/*
BenchmarkMetricKey exercises repeated sided key lookup so the cache stays under
measurement and signal hot paths instead of re-allocating every combine.
*/
func BenchmarkMetricKey(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if MetricKey(MetricArrivalRate, SideBuy) == "" {
			b.Fatal("missing buy metric key")
		}

		if MetricKey(MetricArrivalRate, SideSell) == "" {
			b.Fatal("missing sell metric key")
		}

		if MetricKey(MetricStrength, SideNone) == "" {
			b.Fatal("missing neutral metric key")
		}
	}
}
