package numeric

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPriceSampleRingPush(t *testing.T) {
	Convey("Given a price sample ring", t, func() {
		ring := NewPriceSampleRing(3)
		now := time.Now()

		ring.Push(now, 100)
		ring.Push(now.Add(time.Second), 101)
		ring.Push(now.Add(2*time.Second), 102)
		ring.Push(now.Add(3*time.Second), 103)

		Convey("It should retain the newest samples in order", func() {
			ordered := ring.Ordered()

			So(len(ordered), ShouldEqual, 3)
			So(ordered[0].Price, ShouldEqual, 101)
			So(ordered[2].Price, ShouldEqual, 103)
		})
	})
}

func TestSynchronizedLogReturns(t *testing.T) {
	Convey("Given overlapping price windows", t, func() {
		start := time.Now()
		left := []PriceSample{
			{At: start, Price: 100},
			{At: start.Add(time.Minute), Price: 101},
			{At: start.Add(2 * time.Minute), Price: 102},
		}
		right := []PriceSample{
			{At: start, Price: 200},
			{At: start.Add(time.Minute), Price: 202},
			{At: start.Add(2 * time.Minute), Price: 204},
		}

		leftReturns, rightReturns, ok := SynchronizedLogReturns(left, right, time.Minute)

		Convey("It should emit paired log returns", func() {
			So(ok, ShouldBeTrue)
			So(len(leftReturns), ShouldEqual, len(rightReturns))
			So(len(leftReturns), ShouldBeGreaterThan, 0)
		})
	})
}

func TestHayashiYoshidaCorrelation(t *testing.T) {
	Convey("Given co-moving samples", t, func() {
		start := time.Now()
		left := []PriceSample{
			{At: start, Price: 100},
			{At: start.Add(time.Second), Price: 101},
			{At: start.Add(2 * time.Second), Price: 102},
		}
		right := []PriceSample{
			{At: start, Price: 50},
			{At: start.Add(time.Second), Price: 50.5},
			{At: start.Add(2 * time.Second), Price: 51},
		}

		correlation, ok := HayashiYoshidaCorrelation(left, right)

		Convey("It should estimate positive correlation", func() {
			So(ok, ShouldBeTrue)
			So(correlation, ShouldBeGreaterThan, 0.5)
		})
	})
}

func TestPearson(t *testing.T) {
	Convey("Given perfectly correlated series", t, func() {
		left := []float64{1, 2, 3, 4}
		right := []float64{2, 4, 6, 8}

		Convey("It should return one", func() {
			So(Pearson(left, right), ShouldAlmostEqual, 1, 0.0001)
		})
	})
}

func BenchmarkPriceSampleRingPush(b *testing.B) {
	ring := NewPriceSampleRing(128)
	now := time.Now()

	for b.Loop() {
		ring.Push(now, 100)
	}
}
