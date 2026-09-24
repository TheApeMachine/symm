package store

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReadInterest(t *testing.T) {
	Convey("Given the fields a record resolved", t, func() {
		resolved := map[string]any{
			"trade.data.price":     101.5,
			"trade.data.timestamp": "2026-09-24T10:00:01.5Z",
			"trade.data.side":      "buy",
		}

		Convey("A number is read as itself", func() {
			value, found := readInterest(resolved, "trade.data.price")
			So(found, ShouldBeTrue)
			So(value, ShouldEqual, 101.5)
		})

		Convey("A timestamp is read as the instant it names, in nanoseconds", func() {
			value, found := readInterest(resolved, "trade.data.timestamp")
			So(found, ShouldBeTrue)
			So(value, ShouldEqual, float64(time.Date(2026, 9, 24, 10, 0, 1, 500_000_000, time.UTC).UnixNano()))
		})

		Convey("Other text is not a number", func() {
			_, found := readInterest(resolved, "trade.data.side")
			So(found, ShouldBeFalse)

			Convey("but can still be asked a question", func() {
				value, found := readInterest(resolved, "trade.data.side=buy")
				So(found, ShouldBeTrue)
				So(value, ShouldEqual, 1)
			})
		})
	})
}

func TestWalkInterest(t *testing.T) {
	Convey("Given the same observation live and replayed", t, func() {
		live := map[string]any{
			"channel": "ticker",
			"data":    map[string]any{"last": 101.5},
			"capture": map[string]any{"receivedAt": "2026-09-24T10:00:01Z"},
		}
		replayed := map[string]any{
			"capture": map[string]any{"receivedAt": "2026-09-24T10:00:01Z"},
			"market":  map[string]any{"channel": "ticker", "data": map[string]any{"last": 101.5}},
		}

		Convey("A frame field and a capture field resolve on both", func() {
			for _, record := range []any{live, replayed} {
				last, found := walkInterest(record, []string{"ticker", "data", "last"})
				So(found, ShouldBeTrue)
				So(last, ShouldEqual, 101.5)
				received, found := walkInterest(record, []string{"ticker", "capture", "receivedAt"})
				So(found, ShouldBeTrue)
				So(received, ShouldEqual, "2026-09-24T10:00:01Z")
			}
		})

		Convey("Another channel's field does not resolve", func() {
			_, found := walkInterest(replayed, []string{"trade", "data", "last"})
			So(found, ShouldBeFalse)
		})
	})
}
