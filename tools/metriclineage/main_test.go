package main

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSplitMetricIdentity(t *testing.T) {
	Convey("Given a side-qualified metric wire name", t, func() {
		identity := splitMetricIdentity("liquidity", "depth_ratio:bid")

		Convey("It resolves to the same metric and side identity as a producer", func() {
			So(identity, ShouldResemble, metricID{
				Source: "liquidity",
				Metric: "depth_ratio",
				Side:   "bid",
			})
		})
	})

	Convey("Given an unqualified metric wire name", t, func() {
		identity := splitMetricIdentity("hawkes", "arrival_rate")

		Convey("It retains an empty side", func() {
			So(identity, ShouldResemble, metricID{
				Source: "hawkes",
				Metric: "arrival_rate",
			})
		})
	})
}

func TestBuildReport(t *testing.T) {
	Convey("Given a producer and side-normalized named consumer", t, func() {
		identity := splitMetricIdentity("toxicity", "touch_fill_fraction:ask")
		report := buildReport(
			[]producer{{ID: identity}},
			[]consumerEdge{{
				ID:       identity,
				Kind:     "bound",
				Consumer: "position-risk:executable-liquidity",
			}},
			nil,
		)

		Convey("The producer is referenced rather than falsely reported dead", func() {
			So(report.Summary.TotalProducers, ShouldEqual, 1)
			So(report.Summary.ReferencedProducers, ShouldEqual, 1)
			So(report.Summary.DeadProducers, ShouldEqual, 0)
			So(report.Producers[0].Dead, ShouldBeFalse)
		})
	})
}
