package public

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestIntervalBeginSec(t *testing.T) {
	Convey("Given Kraken interval_begin timestamps", t, func() {
		Convey("It should parse RFC3339 strings to unix seconds", func() {
			sec, err := IntervalBeginSec("2026-05-25T13:54:00Z")

			So(err, ShouldBeNil)
			So(sec, ShouldEqual, time.Date(2026, 5, 25, 13, 54, 0, 0, time.UTC).Unix())
		})

		Convey("It should reject empty interval_begin", func() {
			_, err := IntervalBeginSec("")

			So(err, ShouldNotBeNil)
		})
	})
}

func TestEnrichOhlcWire(t *testing.T) {
	Convey("Given a Kraken ohlc candle map", t, func() {
		candle := map[string]any{
			"symbol":         "BTC/EUR",
			"interval_begin": "2026-05-25T13:54:00Z",
			"open":           1.0,
		}

		Convey("It should add sec for the frontend chart", func() {
			err := EnrichOhlcWire(candle)

			So(err, ShouldBeNil)
			So(candle["sec"], ShouldEqual, time.Date(2026, 5, 25, 13, 54, 0, 0, time.UTC).Unix())
		})
	})
}
