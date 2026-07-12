package types

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestForecastsEligible(t *testing.T) {
	Convey("Given a ready next-event forecast with provenance", t, func() {
		forecast := eligibleForecast()

		Convey("It should be usable by the paper strategy", func() {
			So(forecast.Eligible(), ShouldBeTrue)
		})

		Convey("When readiness is absent", func() {
			forecast.Ready = false

			Convey("Then it is not usable", func() {
				So(forecast.Eligible(), ShouldBeFalse)
			})
		})
	})
}

func TestForecastsExecutableReturn(t *testing.T) {
	Convey("Given a forecast with explicit execution friction", t, func() {
		forecast := eligibleForecast()
		forecast.ExpectedReturn = 0.05
		forecast.ExpectedFees = 0.01
		forecast.ExpectedSpread = 0.005
		forecast.ExpectedImpact = 0.002
		forecast.ExpectedAdverseSelection = 0.003

		Convey("It should subtract every friction component", func() {
			So(forecast.ExecutableReturn(), ShouldAlmostEqual, 0.03)
		})
	})
}

func BenchmarkForecastsExecutableReturn(b *testing.B) {
	forecast := eligibleForecast()

	for b.Loop() {
		_ = forecast.ExecutableReturn()
	}
}

func eligibleForecast() Forecasts {
	return Forecasts{
		Source:        "manifold_forecast",
		Symbol:        "BTC/USD",
		At:            time.Unix(1, 0),
		SourceEpoch:   1,
		HorizonEvents: 1,
		ExpiresEpoch:  2,
		Target:        "next_l3_epoch_mid_log_return",
		ModelVersion:  "test",
		Ready:         true,
		Confidence:    0.5,
	}
}
