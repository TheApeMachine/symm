package trader

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
)

func TestBuildSignalForecast(t *testing.T) {
	config.System.ScalpHoldBeforeExit = 15 * time.Second

	Convey("Given a signal reading and live quote", t, func() {
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
		)

		Convey("It should reject the forecast", func() {
			So(ok, ShouldBeFalse)
		})
	})
}
