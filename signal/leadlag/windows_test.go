package leadlag

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/statistic"
)

func TestWindowsFromCountMatchesZeroSeriesResolve(t *testing.T) {
	Convey("Given windowsFromCount over the full relevant count range", t, func() {
		Convey("When sampleCount is non-positive", func() {
			for _, sampleCount := range []int{-3, 0} {
				short, long, lag := windowsFromCount(sampleCount)

				So(short, ShouldEqual, 0)
				So(long, ShouldEqual, 0)
				So(lag, ShouldEqual, 0)
			}
		})

		Convey("When sampleCount is positive across square and non-square depths", func() {
			for _, sampleCount := range []int{1, 2, 3, 4, 5, 9, 15, 16, 25, 100} {
				sampleCount := sampleCount
				short, long, lag := windowsFromCount(sampleCount)
				wantShort, wantLong, err := statistic.ResolveWindows(
					make([]float64, sampleCount), 0, 0,
				)

				So(err, ShouldBeNil)

				wantLag := max(1, int(math.Ceil(math.Sqrt(float64(wantLong)))))

				if wantLong > 1 {
					wantLag = min(wantLag, wantLong-1)
				}

				So(short, ShouldEqual, wantShort)
				So(long, ShouldEqual, wantLong)
				So(lag, ShouldEqual, wantLag)
			}
		})
	})
}

func BenchmarkWindowsFromCount(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		windowsFromCount(64)
	}
}
