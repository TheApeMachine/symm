package engine

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
)

func TestCalibrationFeedbackFromOHLC(t *testing.T) {
	convey.Convey("Given two completed candles", t, func() {
		candles := map[string][]OHLCCandle{
			"BTC/EUR": {
				{Open: 100, High: 110, Low: 95, Close: 105, Volume: 1},
				{Open: 105, High: 112, Low: 104, Close: 110, Volume: 1},
				{Open: 110, High: 111, Low: 109, Close: 110.5, Volume: 1},
			},
		}

		convey.Convey("It should emit one settled feedback sample", func() {
			feedback := CalibrationFeedbackFromOHLC("pumpdump", candles, 5*time.Minute)
			convey.So(len(feedback), convey.ShouldEqual, 1)
			convey.So(feedback[0].PredictedReturn, convey.ShouldAlmostEqual, 15.0/105.0, 0.0001)
			convey.So(feedback[0].ActualReturn, convey.ShouldAlmostEqual, 5.0/105.0, 0.0001)
		})
	})
}

func TestCompletedCandles(t *testing.T) {
	convey.Convey("Given candles with an in-progress tail", t, func() {
		candles := []OHLCCandle{
			{Close: 1},
			{Close: 2},
			{Close: 3},
		}

		convey.Convey("It should drop the trailing bar", func() {
			completed := CompletedCandles(candles)
			convey.So(len(completed), convey.ShouldEqual, 2)
			convey.So(completed[len(completed)-1].Close, convey.ShouldEqual, 2)
		})
	})
}
