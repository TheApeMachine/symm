package hawkes

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewArrivalStream(t *testing.T) {
	Convey("Given buy and sell arrival times", t, func() {
		start := time.Now()
		stream := NewArrivalStream(
			[]time.Time{start, start.Add(time.Second)},
			[]time.Time{start.Add(500 * time.Millisecond)},
		)

		Convey("It should sort marked events", func() {
			marked := stream.Marked()

			So(len(marked), ShouldEqual, 3)
			So(marked[0].at.Before(marked[1].at), ShouldBeTrue)
		})
	})
}

func TestArrivalStreamFitEventKey(t *testing.T) {
	Convey("Given an arrival stream", t, func() {
		start := time.Now()
		stream := NewArrivalStream(
			[]time.Time{start},
			[]time.Time{start.Add(time.Second)},
		)

		Convey("It should build a stable fit event key", func() {
			key := stream.RevisionKey()

			So(key.buyCount, ShouldEqual, 1)
			So(key.sellCount, ShouldEqual, 1)
		})
	})
}
