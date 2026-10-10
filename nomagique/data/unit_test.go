package data

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCanonicalDimensions(t *testing.T) {
	Convey("Given CanonicalDimensions mapping for metrics", t, func() {
		Convey("price metrics resolve to UnitPrice and TimescaleInstantaneous", func() {
			unit, timescale := CanonicalDimensions("price", "", "")
			So(unit, ShouldEqual, UnitPrice)
			So(timescale, ShouldEqual, TimescaleInstantaneous)

			unit, timescale = CanonicalDimensions("last_price", "", "")
			So(unit, ShouldEqual, UnitPrice)
			So(timescale, ShouldEqual, TimescaleInstantaneous)

			unit, timescale = CanonicalDimensions("best_bid_price", "", "")
			So(unit, ShouldEqual, UnitPrice)
			So(timescale, ShouldEqual, TimescaleInstantaneous)

			unit, timescale = CanonicalDimensions("midpoint", "", "")
			So(unit, ShouldEqual, UnitPrice)
			So(timescale, ShouldEqual, TimescaleInstantaneous)
		})

		Convey("quantity and notional metrics resolve to honest physical dimensions", func() {
			unit, timescale := CanonicalDimensions("qty", "", "")
			So(unit, ShouldEqual, UnitQuantity)
			So(timescale, ShouldEqual, TimescaleInstantaneous)

			unit, timescale = CanonicalDimensions("gross_notional", "", "")
			So(unit, ShouldEqual, UnitNotional)
			So(timescale, ShouldEqual, TimescaleInstantaneous)

			unit, timescale = CanonicalDimensions("trade_notional", "", "")
			So(unit, ShouldEqual, UnitNotional)
			So(timescale, ShouldEqual, TimescaleInstantaneous)
		})

		Convey("correlation metrics resolve to UnitCorrelation and TimescaleRollingWindow", func() {
			unit, timescale := CanonicalDimensions("signed_correlation", UnitDimensionless, "")
			So(unit, ShouldEqual, UnitCorrelation)
			So(timescale, ShouldEqual, TimescaleRollingWindow)

			unit, timescale = CanonicalDimensions("absolute_correlation", "", "")
			So(unit, ShouldEqual, UnitCorrelation)
			So(timescale, ShouldEqual, TimescaleRollingWindow)

			unit, timescale = CanonicalDimensions("signed_correlation@ETH/USD", "", "")
			So(unit, ShouldEqual, UnitCorrelation)
			So(timescale, ShouldEqual, TimescaleRollingWindow)
		})

		Convey("rate metrics resolve to their specific rate units and timescales", func() {
			unit, timescale := CanonicalDimensions("trade_rate", "", "")
			So(unit, ShouldEqual, UnitTradeRate)
			So(timescale, ShouldEqual, TimescaleRollingWindow)

			unit, timescale = CanonicalDimensions("volume_rate", "", "")
			So(unit, ShouldEqual, UnitVolumeRate)
			So(timescale, ShouldEqual, TimescaleVolumeBar)

			unit, timescale = CanonicalDimensions("gross_notional_rate", "", "")
			So(unit, ShouldEqual, UnitNotionalRate)
			So(timescale, ShouldEqual, TimescaleRollingWindow)

			unit, timescale = CanonicalDimensions("arrival_rate", "", "")
			So(unit, ShouldEqual, UnitRate)
			So(timescale, ShouldEqual, TimescaleRollingWindow)
		})

		Convey("covariance and variance resolve to honest physical dimensions", func() {
			unit, timescale := CanonicalDimensions("covariance", "", "")
			So(unit, ShouldEqual, UnitCovariance)
			So(timescale, ShouldEqual, TimescaleRollingWindow)

			unit, timescale = CanonicalDimensions("return_energy:reference", "", "")
			So(unit, ShouldEqual, UnitVariance)
			So(timescale, ShouldEqual, TimescaleRollingWindow)
		})

		Convey("time and duration metrics resolve to UnitSecond or UnitNanosecond", func() {
			unit, timescale := CanonicalDimensions("best_lag_seconds_median", "", "")
			So(unit, ShouldEqual, UnitSecond)
			So(timescale, ShouldEqual, TimescaleRollingWindow)

		})

		Convey("pattern fallbacks resolve unknown metrics by standard suffix", func() {
			unit, timescale := CanonicalDimensions("custom_signal_zscore", "", "")
			So(unit, ShouldEqual, UnitZScore)
			So(timescale, ShouldEqual, TimescaleRollingWindow)

			unit, timescale = CanonicalDimensions("custom_signal_velocity", "", "")
			So(unit, ShouldEqual, UnitVelocity)
			So(timescale, ShouldEqual, TimescaleRollingWindow)

			unit, timescale = CanonicalDimensions("unknown_custom_count", "", "")
			So(unit, ShouldEqual, UnitCount)
			So(timescale, ShouldEqual, TimescaleInstantaneous)
		})

		Convey("explicit units on unknown metrics are preserved", func() {
			unit, timescale := CanonicalDimensions("completely_unknown", UnitSpread, TimescaleEvent)
			So(unit, ShouldEqual, UnitSpread)
			So(timescale, ShouldEqual, TimescaleEvent)
		})
	})
}

func TestCanonicalUnit(t *testing.T) {
	Convey("Given CanonicalUnit", t, func() {
		Convey("it resolves known labels to canonical physical units", func() {
			So(CanonicalUnit("covariance", UnitDimensionless), ShouldEqual, UnitCovariance)
			So(CanonicalUnit("signed_correlation", UnitDimensionless), ShouldEqual, UnitCorrelation)
			So(CanonicalUnit("price", UnitCurrency), ShouldEqual, UnitPrice)
			So(CanonicalUnit("midpoint_response_per_net_notional", ""), ShouldEqual, UnitPriceImpact)
			So(CanonicalUnit("relative_spread", ""), ShouldEqual, UnitRelativeSpread)
			So(CanonicalUnit("excitation_amplitude:buy_from_buy", ""), ShouldEqual, UnitRate)
			So(CanonicalUnit("log_likelihood:hawkes", ""), ShouldEqual, UnitNat)
		})
	})
}

func TestCanonicalTimescale(t *testing.T) {
	Convey("Given CanonicalTimescale", t, func() {
		Convey("it resolves known labels to canonical operational timescales", func() {
			So(CanonicalTimescale("covariance", ""), ShouldEqual, TimescaleRollingWindow)
			So(CanonicalTimescale("price", ""), ShouldEqual, TimescaleInstantaneous)
			So(CanonicalTimescale("advance_count", ""), ShouldEqual, TimescaleRollingWindow)
			So(CanonicalTimescale("measured_return_count", ""), ShouldEqual, TimescaleRollingWindow)
			So(CanonicalTimescale("observation_count", ""), ShouldEqual, TimescaleRollingWindow)
		})
	})
}


func TestSignalMetrics(t *testing.T) {
	Convey("Given SignalMetrics map grouped by signal", t, func() {
		Convey("it contains valid signal sources", func() {
			So(len(SignalMetrics), ShouldBeGreaterThanOrEqualTo, 12)

			correlation, exists := SignalMetrics["correlation"]
			So(exists, ShouldBeTrue)
			So(correlation["covariance_score"].Unit, ShouldEqual, UnitCovarianceScore)
			So(correlation["covariance_score"].Timescale, ShouldEqual, TimescaleRollingWindow)

			cvd, exists := SignalMetrics["cvd"]
			So(exists, ShouldBeTrue)
			So(cvd["trade_count"].Unit, ShouldEqual, UnitCount)

			liquidity, exists := SignalMetrics["liquidity"]
			So(exists, ShouldBeTrue)
			So(liquidity["spread"].Unit, ShouldEqual, UnitSpread)

			ingress, exists := SignalMetrics["ingress"]
			So(exists, ShouldBeTrue)
			So(ingress["price"].Unit, ShouldEqual, UnitPrice)
		})
	})
}
