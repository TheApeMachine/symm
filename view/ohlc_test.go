package view

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/kraken/market"

	. "github.com/smartystreets/goconvey/convey"
)

func TestOHLCFrame(t *testing.T) {
	Convey("Given a candle bar", t, func() {
		ohlc := &OHLC{}
		bar := &market.CandleUpdate{
			Open:          100,
			High:          110,
			Low:           95,
			Close:         105,
			Volume:        12.5,
			IntervalBegin: "2026-05-30T00:00:00Z",
		}

		frame := ohlc.frame("BTC/EUR", bar)

		Convey("It should produce a candle_bar frame matching the wire contract", func() {
			So(frame["event"], ShouldEqual, "candle_bar")
			So(frame["symbol"], ShouldEqual, "BTC/EUR")
			So(frame["open"], ShouldEqual, 100.0)
			So(frame["high"], ShouldEqual, 110.0)
			So(frame["low"], ShouldEqual, 95.0)
			So(frame["close"], ShouldEqual, 105.0)
			So(frame["volume"], ShouldEqual, 12.5)
			So(frame["sec"], ShouldEqual, time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC).Unix())
		})
	})
}

func TestBarSeconds(t *testing.T) {
	Convey("Given interval-begin timestamps", t, func() {
		Convey("It should parse an RFC3339 timestamp to unix seconds", func() {
			So(barSeconds("2026-05-30T00:00:00Z"), ShouldEqual,
				time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC).Unix())
		})

		Convey("It should fall back to now for an empty timestamp", func() {
			before := time.Now().Unix()
			got := barSeconds("")
			So(got, ShouldBeGreaterThanOrEqualTo, before)
		})
	})
}
