package public

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestOhlcSubscribeFrame(t *testing.T) {
	Convey("Given a chart symbol", t, func() {
		frame := OhlcSubscribeFrame("BTC/EUR")

		Convey("It should request Kraken v2 ohlc with snapshot", func() {
			So(frame["method"], ShouldEqual, "subscribe")

			params, ok := frame["params"].(map[string]any)

			So(ok, ShouldBeTrue)
			So(params["channel"], ShouldEqual, CandlesChannel)
			So(params["interval"], ShouldEqual, ChartOhlcIntervalMinutes)
			So(params["snapshot"], ShouldBeTrue)
			So(params["symbol"], ShouldResemble, []string{"BTC/EUR"})
		})
	})
}

func TestCandleIntervalSec(t *testing.T) {
	Convey("Given a Kraken ohlc row", t, func() {
		begin := "2023-10-04T16:25:00.000000000Z"
		expected, parseErr := time.Parse(time.RFC3339Nano, begin)

		So(parseErr, ShouldBeNil)

		sec, err := CandleIntervalSec(OhlcRow{
			Symbol:        "BTC/EUR",
			IntervalBegin: begin,
		})

		Convey("It should map interval_begin to unix seconds", func() {
			So(err, ShouldBeNil)
			So(sec, ShouldEqual, expected.UTC().Unix())
		})
	})

	Convey("Given a row without interval_begin", t, func() {
		_, err := CandleIntervalSec(OhlcRow{Symbol: "BTC/EUR"})

		Convey("It should error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestDecodeOhlc(t *testing.T) {
	Convey("Given an ohlc update envelope", t, func() {
		message := &SocketMessage{
			Channel: CandlesChannel,
			Type:    "update",
			Data: []byte(`[{
				"symbol":"BTC/EUR",
				"open":1,
				"high":2,
				"low":0.5,
				"close":1.5,
				"volume":12.5,
				"interval_begin":"2023-10-04T16:25:00.000000000Z",
				"interval":1
			}]`),
		}

		Convey("It should decode every row", func() {
			rows, err := DecodeOhlc(message)

			So(err, ShouldBeNil)
			So(len(rows), ShouldEqual, 1)
			So(rows[0].Symbol, ShouldEqual, "BTC/EUR")
			So(rows[0].Volume, ShouldEqual, 12.5)
			So(rows[0].Type, ShouldEqual, "update")
		})
	})
}
