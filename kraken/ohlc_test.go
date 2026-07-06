package kraken

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewOHLCDataSlice(t *testing.T) {
	Convey("Given Kraken OHLC data payloads", t, func() {
		payload := []byte(`[{
			"symbol": "ALGO/USD",
			"open": 0.09875,
			"high": 0.0988,
			"low": 0.09875,
			"close": 0.09875,
			"trades": 13,
			"volume": 16255.46368,
			"vwap": 0.09879,
			"interval_begin": "2023-10-04T15:30:00.000000000Z",
			"interval": 5,
			"timestamp": "2023-10-04T15:35:00.000000Z"
		}]`)

		candles := NewOHLCDataSlice(payload)

		Convey("It should decode the candle fields", func() {
			So(len(candles), ShouldEqual, 1)

			ohlc := candles[0]

			So(ohlc.Symbol, ShouldEqual, "ALGO/USD")
			So(ohlc.Interval, ShouldEqual, 5)
			So(ohlc.Trades, ShouldEqual, 13)
			So(ohlc.Vwap, ShouldAlmostEqual, 0.09879)
			So(ohlc.IntervalBegin.IsZero(), ShouldBeFalse)
		})
	})
}
