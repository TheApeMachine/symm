package types

import (
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

/*
TestNewMarketFeed verifies that both bounded ring capacities are validated
before live ingestion can discover a lazy per-symbol track configuration error.
*/
func TestNewMarketFeed(t *testing.T) {
	Convey("Given invalid market feed capacities", t, func() {
		Convey("A non-power-of-two timeline is rejected", func() {
			feed := NewMarketFeed[marketFeedRow](3, 4)
			_, err := feed.Pending(time.Now().UTC())

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "timeline capacity")
		})

		Convey("A non-power-of-two symbol track is rejected", func() {
			feed := NewMarketFeed[marketFeedRow](4, 3)
			_, err := feed.Pending(time.Now().UTC())

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "track capacity")
		})
	})
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

func TestMarketFeed_Progress(t *testing.T) {
	Convey("Given a captured feed with and without new ingress", t, func() {
		feed := NewMarketFeed[marketFeedRow](4, 4)
		start := time.Unix(100, 0).UTC()
		So(feed.Capture(start), ShouldBeNil)
		So(feed.Progress(), ShouldBeFalse)
		So(feed.Observe("BTC/USD", start, marketFeedRow{value: 1}), ShouldBeNil)
		So(feed.Capture(start), ShouldBeNil)
		So(feed.Progress(), ShouldBeTrue)
		_, err := feed.Frame(start)
		So(err, ShouldBeNil)
		So(feed.Capture(start), ShouldBeNil)
		So(feed.Progress(), ShouldBeFalse)
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

func TestMarketFeed_Drain(t *testing.T) {
	Convey("Given more unseen path events than the timeline can retain", t, func() {
		feed := NewMarketFeed[marketFeedRow](2, 2)
		start := time.Unix(100, 0).UTC()

		for index := range 3 {
			So(feed.Observe("BTC/USD", start, marketFeedRow{value: index}), ShouldBeNil)
		}

		rows, err := feed.Drain(start)

		Convey("It consumes the retained window and advances the cursor", func() {
			So(err, ShouldBeNil)
			So(rows, ShouldHaveLength, 2)
			So(rows[0].value, ShouldEqual, 1)
			So(rows[1].value, ShouldEqual, 2)
			So(feed.cursor.After, ShouldEqual, 3)
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
