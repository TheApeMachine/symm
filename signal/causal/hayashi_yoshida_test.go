package causal

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHYReturnsObserve(t *testing.T) {
	Convey("Given asynchronous trade prints", t, func() {
		series := newHYReturns(8)
		base := int64(time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC).UnixNano())

		series.Observe(base, 100)
		series.Observe(base+int64(time.Millisecond), 101)
		series.Observe(base+int64(2*time.Millisecond), 102)

		Convey("It should accumulate log-return intervals", func() {
			So(series.len(), ShouldEqual, 2)
			So(series.realisedVariance(), ShouldBeGreaterThan, 0)
		})
	})
}

func TestHayashiYoshidaCorrelation(t *testing.T) {
	Convey("Given two co-moving return series", t, func() {
		base := int64(time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC).UnixNano())
		left := correlatedHYSeries(base, []float64{100, 101, 102, 103, 104})
		right := correlatedHYSeries(base, []float64{200, 202, 204, 206, 208})

		correlation, ok := hayashiYoshidaCorrelation(left, right)

		Convey("It should report a positive correlation", func() {
			So(ok, ShouldBeTrue)
			So(correlation, ShouldBeGreaterThan, 0.5)
		})
	})
}

func TestHYReturnsOutOfOrder(t *testing.T) {
	Convey("Given out-of-order trade timestamps", t, func() {
		series := newHYReturns(4)
		base := int64(time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC).UnixNano())

		series.Observe(base, 100)
		series.Observe(base+int64(time.Millisecond), 101)
		series.Observe(base, 102)

		Convey("It should advance the anchor without duplicating intervals", func() {
			So(series.len(), ShouldEqual, 1)
		})
	})
}

func TestHayashiYoshidaCorrelationQuietBook(t *testing.T) {
	Convey("Given a flat price series", t, func() {
		base := int64(time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC).UnixNano())
		left := correlatedHYSeries(base, []float64{100, 100, 100})
		right := correlatedHYSeries(base, []float64{200, 200, 200})

		_, ok := hayashiYoshidaCorrelation(left, right)

		Convey("It should refuse zero-variance correlation", func() {
			So(ok, ShouldBeFalse)
		})
	})
}

func BenchmarkHayashiYoshidaCorrelation(b *testing.B) {
	base := int64(time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC).UnixNano())
	left := correlatedHYSeries(base, []float64{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115})
	right := correlatedHYSeries(base, []float64{200, 202, 204, 206, 208, 210, 212, 214, 216, 218, 220, 222, 224, 226, 228, 230})

	b.ReportAllocs()

	for b.Loop() {
		_, _ = hayashiYoshidaCorrelation(left, right)
	}
}
