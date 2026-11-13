package main

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSplitMetricIdentity(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When the wire name is side-qualified", func() {
			identity := splitMetricIdentity("liquidity", "depth_ratio:bid")

			Convey("Source, metric, and side are split", func() {
				So(identity, ShouldResemble, metricID{
					Source: "liquidity",
					Metric: "depth_ratio",
					Side:   "bid",
				})
			})
		})

		Convey("When the wire name has no side", func() {
			identity := splitMetricIdentity("hawkes", "arrival_rate")

			Convey("Side stays empty", func() {
				So(identity, ShouldResemble, metricID{
					Source: "hawkes",
					Metric: "arrival_rate",
				})
			})
		})
	})
}

func TestMetricIDString(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When side is set", func() {
			id := metricID{Source: "liquidity", Metric: "depth_ratio", Side: "bid"}

			Convey("String uses source/metric:side", func() {
				So(id.String(), ShouldEqual, "liquidity/depth_ratio:bid")
			})
		})

		Convey("When side is empty", func() {
			id := metricID{Source: "hawkes", Metric: "arrival_rate"}

			Convey("String uses source/metric", func() {
				So(id.String(), ShouldEqual, "hawkes/arrival_rate")
			})
		})
	})
}

func TestBuildReport(t *testing.T) {
	Convey("Setup", t, func() {
		identity := splitMetricIdentity("toxicity", "touch_fill_fraction:ask")

		Convey("When the producer is referenced by a bound consumer", func() {
			report := buildReport(
				[]producer{{ID: identity}},
				[]consumerEdge{{
					ID:       identity,
					Kind:     "bound",
					Consumer: "position-risk:executable-liquidity",
				}},
				nil,
			)

			Convey("The producer is referenced, not dead", func() {
				So(report.Summary.TotalProducers, ShouldEqual, 1)
				So(report.Summary.ReferencedProducers, ShouldEqual, 1)
				So(report.Summary.DeadProducers, ShouldEqual, 0)
				So(report.Producers[0].Dead, ShouldBeFalse)
			})
		})
	})
}

func BenchmarkSplitMetricIdentity(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = splitMetricIdentity("liquidity", "depth_ratio:bid")
	}
}

func BenchmarkMetricIDString(b *testing.B) {
	id := metricID{Source: "liquidity", Metric: "depth_ratio", Side: "bid"}

	b.ReportAllocs()

	for b.Loop() {
		_ = id.String()
	}
}

func BenchmarkBuildReport(b *testing.B) {
	identity := splitMetricIdentity("toxicity", "touch_fill_fraction:ask")
	producers := []producer{{ID: identity}}
	edges := []consumerEdge{{
		ID:       identity,
		Kind:     "bound",
		Consumer: "position-risk:executable-liquidity",
	}}

	b.ReportAllocs()

	for b.Loop() {
		_ = buildReport(producers, edges, nil)
	}
}
