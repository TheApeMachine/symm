package types

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
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

		Convey("When resolved skill evidence is absent", func() {
			forecast.CalibrationSamples = 0
			forecast.IncrementalSkillLowerBound = 0

			Convey("Then a calibrated label cannot make it usable", func() {
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

/*
BenchmarkForecastsEligible measures the complete forecast provenance and
calibration boundary used before strategy selection.
*/
func BenchmarkForecastsEligible(b *testing.B) {
	forecast := eligibleForecast()

	for b.Loop() {
		_ = forecast.Eligible()
	}
}

func eligibleForecast() Forecasts {
	return Forecasts{
		Source:                     "manifold_forecast",
		Symbol:                     "BTC/USD",
		At:                         time.Unix(1, 0),
		SourceEpoch:                1,
		HorizonEvents:              1,
		ExpiresEpoch:               2,
		Target:                     "next_l3_epoch_mid_log_return",
		ModelVersion:               "test",
		Ready:                      true,
		Calibrated:                 true,
		FrictionReady:              true,
		CalibrationSamples:         8,
		IncrementalSkillLowerBound: 0.0001,
		ReferencePrice:             decimal.NewFromInt64(100),
		BuyCapacity:                decimal.NewFromInt64(1000),
		SellCapacity:               decimal.NewFromInt64(1000),
		Confidence:                 0.5,
	}
}
