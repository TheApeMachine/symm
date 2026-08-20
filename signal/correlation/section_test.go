package correlation

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestScores(t *testing.T) {
	Convey("Given asynchronous correlated cohort paths", t, func() {
		section := NewSection()
		start := time.Unix(1_700_000_000, 0).UTC()

		for index, price := range []float64{100, 101, 103, 106} {
			section.Observe("AAA/USD", price, start.Add(time.Duration(index)*time.Second))
			section.Observe("BBB/USD", price*2, start.Add(time.Duration(index)*time.Second+time.Millisecond))
		}

		scores, ready := section.Scores("AAA/USD")

		Convey("It should derive supported cohort evidence", func() {
			So(ready, ShouldBeTrue)
			So(scores.Correlation, ShouldBeGreaterThan, 0)
			So(scores.Herd, ShouldBeGreaterThan, 0)
			So(scores.Herd, ShouldBeLessThanOrEqualTo, 1)
		})
	})
}

func BenchmarkScores(benchmark *testing.B) {
	section := NewSection()
	start := time.Unix(1_700_000_000, 0).UTC()

	for index := range 64 {
		at := start.Add(time.Duration(index) * time.Second)
		section.Observe("AAA/USD", 100+float64(index), at)
		section.Observe("BBB/USD", 200+float64(index)*2, at.Add(time.Millisecond))
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		section.Scores("AAA/USD")
	}
}
