package correlation

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHayashiYoshida(t *testing.T) {
	Convey("Given two asynchronously sampled proportional paths", t, func() {
		left := []Sample{
			{At: time.Unix(0, 0), Value: 100},
			{At: time.Unix(1, 0), Value: 110},
			{At: time.Unix(2, 0), Value: 121},
			{At: time.Unix(3, 0), Value: 133.1},
		}
		right := []Sample{
			{At: time.Unix(0, int64(time.Millisecond)), Value: 50},
			{At: time.Unix(1, int64(time.Millisecond)), Value: 55},
			{At: time.Unix(2, int64(time.Millisecond)), Value: 60.5},
			{At: time.Unix(3, int64(time.Millisecond)), Value: 66.55},
		}

		value, ready := HayashiYoshida(left, right, 0)

		Convey("It should correlate overlapping return intervals", func() {
			So(ready, ShouldBeTrue)
			So(math.Abs(value-1), ShouldBeLessThan, 1e-9)
		})
	})
}

func BenchmarkHayashiYoshida(benchmark *testing.B) {
	left := make([]Sample, 128)
	right := make([]Sample, 128)

	for index := range left {
		left[index] = Sample{At: time.Unix(int64(index), 0), Value: 100 + float64(index)}
		right[index] = Sample{At: time.Unix(int64(index), int64(time.Millisecond)), Value: 200 + 2*float64(index)}
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		HayashiYoshida(left, right, 0)
	}
}
