package trader

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
)

func seedReturnModel(source, regime string, actual float64) *ReturnModel {
	model := NewReturnModel()

	for range config.System.MinCalibrationSamples {
		model.Apply(engine.PredictionFeedback{
			Source:          source,
			Regime:          regime,
			PredictedReturn: 0.01,
			ActualReturn:    actual,
		})
	}

	return model
}

func TestBuildSignalForecast(t *testing.T) {
	config.System.ScalpHoldBeforeExit = 15 * time.Second
	model := seedReturnModel("hawkes", "momentum", 0.02)

	Convey("Given calibrated forward returns", t, func() {
		measurement := engine.Measurement{
			Source:     "hawkes",
			Regime:     "momentum",
			Reason:     "cluster_buy",
			Confidence: 0.5,
		}

		forecast, ok := BuildSignalForecast(
			measurement,
			stubPrices{"PUMP/EUR": 100},
			"PUMP/EUR",
			model,
		)

		Convey("It should derive trader-owned expected return and runway", func() {
			So(ok, ShouldBeTrue)
			So(forecast.Runway, ShouldEqual, 15*time.Second)
			So(forecast.ExpectedReturn, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given zero confidence", t, func() {
		_, ok := BuildSignalForecast(
			engine.Measurement{Confidence: 0},
			stubPrices{"PUMP/EUR": 100},
			"PUMP/EUR",
			model,
		)

		Convey("It should reject the forecast", func() {
			So(ok, ShouldBeFalse)
		})
	})
}

func TestReturnModelPredictRequiresSamples(t *testing.T) {
	model := NewReturnModel()

	_, ok := model.Predict("hawkes", "momentum", 0.5)

	if ok {
		t.Fatal("expected forecast to require calibration samples")
	}
}
