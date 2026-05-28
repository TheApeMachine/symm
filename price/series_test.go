package price

import (
	"testing"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMeasurementRunway(t *testing.T) {
	Convey("Given an invalid measurement timeframe", t, func() {
		measurement := engine.Measurement{
			Type: engine.Momentum,
			Timeframe: engine.Timeframe{
				Start: 20,
				End:   10,
			},
		}

		Convey("It should use the configured fallback runway", func() {
			So(measurementRunway(measurement), ShouldEqual, config.System.ScalpHoldBeforeExit)
		})
	})
}
