package config

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestDerivedRegimeSpec(t *testing.T) {
	Convey("Given gauge cadence and a symbol universe", t, func() {
		viper.Set("telemetry.gauge.publish_interval", 100*time.Millisecond)
		viper.Set("market.default_symbols", []string{"BTC/USD", "ETH/USD"})

		regime, err := DerivedRegimeSpec()

		Convey("It should derive a power-of-two window and warmup count", func() {
			So(err, ShouldBeNil)
			So(regime.Window, ShouldBeGreaterThanOrEqualTo, 64)
			So(regime.Window&(regime.Window-1), ShouldEqual, 0)
			So(regime.MinSamples, ShouldBeGreaterThanOrEqualTo, 4)
		})
	})
}

func TestDerivedBaselineSpec(t *testing.T) {
	Convey("Given a derived regime window", t, func() {
		baseline := DerivedBaselineSpec(RegimeSpec{Window: 256, MinSamples: 8})

		Convey("It should produce ordered sigma and alpha bounds", func() {
			So(baseline.AlphaMax, ShouldBeGreaterThan, baseline.AlphaMin)
			So(baseline.StrongTrendSigma, ShouldBeGreaterThan, baseline.TrendSigma)
			So(baseline.VolFloorSigma, ShouldBeGreaterThan, baseline.StrongTrendSigma)
		})
	})
}
