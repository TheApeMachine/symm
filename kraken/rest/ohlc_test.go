package rest

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestParseOHLCCandle(t *testing.T) {
	convey.Convey("Given one Kraken OHLC row", t, func() {
		row := []any{
			1565551200.0,
			"9500.0",
			"9600.0",
			"9400.0",
			"9550.0",
			"9525.0",
			"12.5",
			42.0,
		}

		convey.Convey("It should parse OHLCV fields", func() {
			candle, ok := parseCandle(row)
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(candle.Open, convey.ShouldEqual, 9500)
			convey.So(candle.High, convey.ShouldEqual, 9600)
			convey.So(candle.Low, convey.ShouldEqual, 9400)
			convey.So(candle.Close, convey.ShouldEqual, 9550)
			convey.So(candle.Volume, convey.ShouldEqual, 12.5)
		})
	})
}

func TestParseOHLCCandlesFromResult(t *testing.T) {
	convey.Convey("Given a Kraken OHLC payload", t, func() {
		payload := ohlcResponse{
			Result: map[string]any{
				"XXBTZEUR": []any{
					[]any{1565551200.0, "1", "2", "0.5", "1.5", "1.2", "10", 1.0},
				},
				"last": 1565551200.0,
			},
		}

		convey.Convey("It should extract candle rows", func() {
			rows, err := payload.candleRows()
			convey.So(err, convey.ShouldBeNil)
			convey.So(len(parseCandles(rows)), convey.ShouldEqual, 1)
		})
	})
}
