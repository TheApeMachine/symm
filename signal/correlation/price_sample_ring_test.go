package correlation

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPriceSampleRingPush(t *testing.T) {
	Convey("Given a price sample ring", t, func() {
		ring := NewPriceSampleRing(3)
		at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		ring.Push(at, 100)
		ring.Push(at.Add(time.Second), 101)
		ring.Push(at.Add(2*time.Second), 102)
		ring.Push(at.Add(3*time.Second), 103)

		ordered := ring.Ordered()

		Convey("It should evict the oldest sample at capacity", func() {
			So(len(ordered), ShouldEqual, 3)
			So(ordered[0].Price, ShouldAlmostEqual, 101, 1e-12)
			So(ordered[2].Price, ShouldAlmostEqual, 103, 1e-12)
		})

		Convey("It should ignore invalid pushes", func() {
			empty := NewPriceSampleRing(0)
			empty.Push(time.Time{}, 100)
			empty.Push(at, 0)
			So(len(empty.Ordered()), ShouldEqual, 0)
		})
	})
}

func BenchmarkPriceSampleRingPush(b *testing.B) {
	ring := NewPriceSampleRing(256)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for b.Loop() {
		ring.Push(at, 100)
	}
}
