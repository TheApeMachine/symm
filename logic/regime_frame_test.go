package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMarketRegimeFrame(testingTB *testing.T) {
	Convey("Given publishable measurements", testingTB, func() {
		frame := MarketRegimeFrame([]Measurement{{
			Source:     SourceFluid,
			Symbol:     "BTC/EUR",
			Price:      100,
			Spread:     1,
			Strength:   0.8,
			Confidence: 0.7,
			Surprise:   0.2,
			Category:   CategoryVerticalIgnition,
		}})

		Convey("It should publish market regime axes", func() {
			So(frame["type"], ShouldEqual, "regime")
			So(frame["symbol"], ShouldEqual, "market")
			So(frame["volatility"], ShouldBeGreaterThan, 0)
			So(frame["trend"], ShouldBeGreaterThan, 0)
		})
	})
}
