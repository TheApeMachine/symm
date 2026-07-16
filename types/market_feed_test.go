package types

import (
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/structure"
)

type marketFeedRow struct {
	symbol string
	at     time.Time
	value  int
}

func TestMarketFeed_Frame(t *testing.T) {
	Convey("Given symbols that update at different rates", t, func() {
		feed := NewMarketFeed[marketFeedRow](8, 4)
		start := time.Unix(100, 0).UTC()
		So(feed.Observe("BTC/USD", start, marketFeedRow{"BTC/USD", start, 1}), ShouldBeNil)
		So(feed.Observe("ETH/USD", start, marketFeedRow{"ETH/USD", start, 2}), ShouldBeNil)
		first, err := feed.Frame(start)
		So(err, ShouldBeNil)
		So(first, ShouldHaveLength, 2)
		So(feed.Observe(
			"BTC/USD",
			start.Add(time.Second),
			marketFeedRow{"BTC/USD", start.Add(time.Second), 3},
		), ShouldBeNil)

		second, err := feed.Frame(start.Add(time.Second))

		Convey("It carries the slow symbol's newest real state into the next frame", func() {
			So(err, ShouldBeNil)
			So(second, ShouldHaveLength, 2)
			So(second[0].value, ShouldEqual, 2)
			So(second[1].value, ShouldEqual, 3)
		})
	})
}

func TestMarketFeed_Batch(t *testing.T) {
	Convey("Given a non-destructive event batch", t, func() {
		feed := NewMarketFeed[marketFeedRow](4, 4)
		start := time.Unix(100, 0).UTC()
		So(feed.Observe("BTC/USD", start, marketFeedRow{value: 1}), ShouldBeNil)
		batch, err := feed.Batch(start)
		So(err, ShouldBeNil)
		pending, err := feed.Pending(start)
		So(err, ShouldBeNil)

		Convey("It remains pending until the caller commits processing", func() {
			So(batch.Rows, ShouldHaveLength, 1)
			So(pending, ShouldHaveLength, 1)
			So(feed.Commit(batch), ShouldBeNil)
			pending, err = feed.Pending(start)
			So(err, ShouldBeNil)
			So(pending, ShouldBeEmpty)
		})
	})
}

func TestMarketFeed_Overrun(t *testing.T) {
	Convey("Given more unseen path events than the timeline can retain", t, func() {
		feed := NewMarketFeed[marketFeedRow](2, 2)
		start := time.Unix(100, 0).UTC()

		for index := range 3 {
			So(feed.Observe("BTC/USD", start, marketFeedRow{value: index}), ShouldBeNil)
		}

		_, err := feed.Drain(start)
		overrun := structure.ClockOverrunError{}

		Convey("It exposes the overrun without advancing the cursor", func() {
			So(errors.As(err, &overrun), ShouldBeTrue)
			So(overrun.Expected, ShouldEqual, 1)
			So(overrun.Oldest, ShouldEqual, 2)
		})
	})
}

func BenchmarkMarketFeed_Drain8192(b *testing.B) {
	start := time.Unix(100, 0).UTC()
	feed := NewMarketFeed[marketFeedRow](8192, 128)

	for index := range 8192 {
		if err := feed.Observe(
			"symbol",
			start,
			marketFeedRow{value: index},
		); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		feed.cursor = structure.ClockCursor{}

		if _, err := feed.Drain(start); err != nil {
			b.Fatal(err)
		}
	}
}
