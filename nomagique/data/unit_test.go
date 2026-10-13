package data

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetDimension(t *testing.T) {
	Convey("Given GetDimension", t, func() {
		Convey("it resolves known producer metrics", func() {
			unit, timescale := GetDimension("spot:trade", "price")
			So(unit, ShouldEqual, UnitPrice)
			So(timescale, ShouldEqual, TimescaleInstantaneous)

			unit, timescale = GetDimension("correlation", "cohort_covariance_score")
			So(unit, ShouldEqual, UnitCovarianceScore)
			So(timescale, ShouldEqual, TimescaleRollingWindow)

			unit, timescale = GetDimension("cvd", "signed_net_fraction")
			So(unit, ShouldEqual, UnitImbalance)
			So(timescale, ShouldEqual, TimescaleRollingWindow)
		})

		Convey("it resolves scoped metrics by stripping index", func() {
			unit, timescale := GetDimension("spot:trade", "bid_price_0")
			So(unit, ShouldEqual, UnitPrice)
			So(timescale, ShouldEqual, TimescaleInstantaneous)
		})

		Convey("it returns UnitNone and TimescaleNone for unknown producers or metrics", func() {
			unit, timescale := GetDimension("unknown_producer", "price")
			So(unit, ShouldEqual, UnitNone)
			So(timescale, ShouldEqual, TimescaleNone)

			unit, timescale = GetDimension("spot:trade", "unknown_metric")
			So(unit, ShouldEqual, UnitNone)
			So(timescale, ShouldEqual, TimescaleNone)

			unit, timescale = GetDimension("ingress", "price")
			So(unit, ShouldEqual, UnitNone)
			So(timescale, ShouldEqual, TimescaleNone)
		})
	})
}

func TestSignalMetrics(t *testing.T) {
	Convey("Given SignalMetrics map grouped by signal", t, func() {
		Convey("it contains valid signal sources", func() {
			So(len(SignalMetrics), ShouldBeGreaterThanOrEqualTo, 12)

			correlation, exists := SignalMetrics["correlation"]
			So(exists, ShouldBeTrue)
			So(correlation["cohort_covariance_score"].Unit, ShouldEqual, UnitCovarianceScore)
			So(correlation["cohort_covariance_score"].Timescale, ShouldEqual, TimescaleRollingWindow)

			cvd, exists := SignalMetrics["cvd"]
			So(exists, ShouldBeTrue)
			So(cvd["signed_net_fraction"].Unit, ShouldEqual, UnitImbalance)

			liquidity, exists := SignalMetrics["liquidity"]
			So(exists, ShouldBeTrue)
			So(liquidity["relative_spread"].Unit, ShouldEqual, UnitRelativeSpread)

			spotTrade, exists := SignalMetrics["spot:trade"]
			So(exists, ShouldBeTrue)
			So(spotTrade["price"].Unit, ShouldEqual, UnitPrice)

			spotLevel3, exists := SignalMetrics["spot:level3"]
			So(exists, ShouldBeTrue)
			So(spotLevel3["limit_price"].Unit, ShouldEqual, UnitPrice)

			_, ingressExists := SignalMetrics["ingress"]
			So(ingressExists, ShouldBeFalse)

			_, level3Exists := SignalMetrics["level3"]
			So(level3Exists, ShouldBeFalse)
		})
	})
}
