package types

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestOrderEvents(t *testing.T) {
	Convey("Given asynchronous entity events with equal-time ties", t, func() {
		start := time.Unix(100, 0).UTC()
		events := []Event{
			{Stream: "book", Priority: 2, Sequence: 1, At: start.Add(time.Second)},
			{Stream: "trade", Priority: 1, Sequence: 2, At: start},
			{Stream: "ticker", Priority: 0, Sequence: 1, At: start},
			{Stream: "trade", Priority: 1, Sequence: 1, At: start},
		}

		OrderEvents(events)

		Convey("It orders by time, priority, and stream sequence", func() {
			So(events[0].Stream, ShouldEqual, "ticker")
			So(events[1].Sequence, ShouldEqual, 1)
			So(events[2].Sequence, ShouldEqual, 2)
			So(events[3].Stream, ShouldEqual, "book")
		})
	})
}

func BenchmarkOrderEvents(b *testing.B) {
	start := time.Unix(100, 0).UTC()
	template := make([]Event, 384)

	for index := range template {
		template[index] = Event{
			Stream:   []string{"ticker", "trade", "book"}[index%3],
			Priority: index % 3,
			Sequence: uint64(index/3 + 1),
			At:       start.Add(time.Duration(len(template) - index)),
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		events := append([]Event(nil), template...)
		OrderEvents(events)
	}
}
