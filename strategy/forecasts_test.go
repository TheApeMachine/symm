package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestSelectForecastsPrefersNewerEpoch(t *testing.T) {
	Convey("Given two forecasts for one symbol", t, func() {
		rows := []types.Forecasts{
			{Symbol: "MATIC/USD", SourceEpoch: 1, Ready: true, Calibrated: true, FrictionReady: true, ExpectedReturn: 0.01},
			{Symbol: "MATIC/USD", SourceEpoch: 2, Ready: true, Calibrated: true, FrictionReady: true, ExpectedReturn: 0.09},
		}

		Convey("When selectForecasts runs", func() {
			selected := selectForecasts(rows)

			Convey("Then the newer epoch wins", func() {
				So(selected["MATIC/USD"].SourceEpoch, ShouldEqual, 2)
				So(selected["MATIC/USD"].ExpectedReturn, ShouldEqual, 0.09)
			})
		})
	})

	Convey("Given equal-epoch forecasts where only one is eligible", t, func() {
		eligible := selectTestForecast("MATIC/USD", 1, 0.09)
		ineligible := selectTestForecast("MATIC/USD", 1, 0.01)
		ineligible.Ready = false
		rows := []types.Forecasts{ineligible, eligible}

		Convey("When selectForecasts runs", func() {
			selected := selectForecasts(rows)

			Convey("Then the eligible forecast wins", func() {
				So(selected["MATIC/USD"].Eligible(), ShouldBeTrue)
				So(selected["MATIC/USD"].ExpectedReturn, ShouldEqual, 0.09)
			})
		})
	})

	Convey("Given equal-priority forecasts", t, func() {
		first := selectTestForecast("MATIC/USD", 1, 0.01)
		second := selectTestForecast("MATIC/USD", 1, 0.09)
		rows := []types.Forecasts{first, second}

		Convey("When selectForecasts runs", func() {
			selected := selectForecasts(rows)

			Convey("Then the first-seen forecast is retained", func() {
				So(selected["MATIC/USD"].ExpectedReturn, ShouldEqual, 0.01)
			})
		})
	})

	Convey("Given no forecast for the requested symbol", t, func() {
		rows := []types.Forecasts{
			selectTestForecast("MATIC/USD", 1, 0.01),
		}

		Convey("When selectForecast looks up an absent symbol", func() {
			_, found := selectForecast(rows, "PENGU/USD")

			Convey("Then the lookup reports absent", func() {
				So(found, ShouldBeFalse)
			})
		})
	})
}

func BenchmarkSelectForecasts(b *testing.B) {
	rows := []types.Forecasts{
		selectTestForecast("MATIC/USD", 1, 0.01),
		selectTestForecast("MATIC/USD", 2, 0.09),
		selectTestForecast("PENGU/USD", 1, 0.04),
		{Symbol: "SKIP/USD", SourceEpoch: 1, Ready: false, ExpectedReturn: 0.02},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = selectForecasts(rows)
	}
}

func selectTestForecast(symbol string, epoch uint64, expectedReturn float64) types.Forecasts {
	return types.Forecasts{
		Source:                     "manifold_forecast",
		Symbol:                     symbol,
		At:                         time.Unix(1, 0),
		SourceEpoch:                epoch,
		HorizonEvents:              1,
		ExpiresEpoch:               epoch + 1,
		Target:                     "next_l3_epoch_mid_log_return",
		ModelVersion:               "test",
		Ready:                      true,
		Calibrated:                 true,
		FrictionReady:              true,
		CalibrationSamples:         8,
		IncrementalSkillLowerBound: 0.0001,
		ReferencePrice:             100,
		BuyCapacity:                1000,
		SellCapacity:               1000,
		Confidence:                 0.5,
		ExpectedReturn:             expectedReturn,
	}
}
