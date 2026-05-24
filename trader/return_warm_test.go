package trader

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
)

func TestWarmFromOHLC(t *testing.T) {
	Convey("Given completed OHLC candles with positive forward returns", t, func() {
		previousMinSamples := config.System.MinCalibrationSamples
		config.System.MinCalibrationSamples = 2
		defer func() { config.System.MinCalibrationSamples = previousMinSamples }()

		model := NewReturnModel()
		candles := map[string][]engine.OHLCCandle{
			"BTC/EUR": {
				{Time: time.Unix(1, 0), Open: 100, High: 104, Low: 99, Close: 100, Volume: 1},
				{Time: time.Unix(2, 0), Open: 101, High: 106, Low: 100, Close: 102, Volume: 1},
				{Time: time.Unix(3, 0), Open: 103, High: 108, Low: 102, Close: 105, Volume: 1},
				{Time: time.Unix(4, 0), Open: 106, High: 109, Low: 105, Close: 107, Volume: 1},
			},
		}

		warmed := model.WarmFromOHLC("basis", []string{"basis"}, candles)
		gross, ok := model.Predict("basis", "basis", 0.5)

		Convey("It should seed return buckets for live candidate forecasts", func() {
			So(warmed, ShouldEqual, 2)
			So(ok, ShouldBeTrue)
			So(gross, ShouldBeGreaterThan, 0)
		})
	})
}

func TestWarmActualReturn(t *testing.T) {
	Convey("Given a downward move", t, func() {
		longReturn := warmActualReturn("momentum", 100, 95)
		dumpReturn := warmActualReturn("dump", 100, 95)

		Convey("It should only score the dump regime positively", func() {
			So(longReturn, ShouldBeLessThan, 0)
			So(dumpReturn, ShouldBeGreaterThan, 0)
		})
	})
}
