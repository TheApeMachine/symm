package market

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCandleParams(t *testing.T) {
	Convey("Given candle subscribe params", t, func() {
		params := CandleParams{
			Channel:  "ohlc",
			Symbol:   []string{"BTC/EUR"},
			Interval: 1,
			Snapshot: true,
		}

		Convey("It should retain the configured interval", func() {
			So(params.Interval, ShouldEqual, 1)
			So(params.Snapshot, ShouldBeTrue)
		})
	})
}

func TestCandleUpdateFields(t *testing.T) {
	Convey("Given a candle update row", t, func() {
		update := CandleUpdate{
			Symbol:        "BTC/EUR",
			Open:          1,
			Close:         2,
			IntervalBegin: time.Now().UTC().Format(time.RFC3339Nano),
		}

		Convey("It should expose symbol and close", func() {
			So(update.Symbol, ShouldEqual, "BTC/EUR")
			So(update.Close, ShouldEqual, 2)
		})
	})
}
