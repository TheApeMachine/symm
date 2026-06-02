package market

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTickerUpdateSetEnvelopeType(t *testing.T) {
	Convey("Given a ticker update", t, func() {
		ticker := TickerUpdate{Symbol: "BTC/EUR", Last: 100}

		ticker.SetEnvelopeType("snapshot")

		Convey("It should record the envelope type", func() {
			So(ticker.Type, ShouldEqual, "snapshot")
		})
	})
}

func TestOrderEventTime(t *testing.T) {
	Convey("Given a valid RFC3339 timestamp", t, func() {
		fallback := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		parsed := OrderEventTime("2026-06-02T12:00:00Z", fallback)

		Convey("It should parse the timestamp", func() {
			So(parsed.UTC().Format(time.RFC3339), ShouldEqual, "2026-06-02T12:00:00Z")
		})
	})
}
