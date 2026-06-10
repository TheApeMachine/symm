package adaptive

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBaselineObserve(t *testing.T) {
	Convey("Given a baseline tracker", t, func() {
		baseline := NewBaseline(0.001, 4)

		Convey("It should reject windows below min observations", func() {
			_ = baseline.Observe(0.01, 0.1)
			_ = baseline.Observe(0.012, 0.1)

			_, ready := baseline.Threshold(2)

			So(ready, ShouldBeFalse)
		})

		Convey("It should expose a sigma threshold once warmed", func() {
			for range 8 {
				_ = baseline.Observe(0.01, 0.1)
			}

			threshold, ready := baseline.Threshold(2)

			So(ready, ShouldBeTrue)
			So(threshold, ShouldBeGreaterThan, baseline.Mean())
		})

		Convey("It should re-center after a baseline shift", func() {
			tracker := NewBaseline(0.001, 4)

			for range 32 {
				_ = tracker.Observe(0.01, 0.05)
			}

			for range 32 {
				_ = tracker.Observe(0.05, 0.25)
			}

			zScore, ready := tracker.ZScore(0.05, 0)

			So(ready, ShouldBeTrue)
			So(zScore, ShouldBeBetween, -1, 1)
		})
	})
}

func BenchmarkBaselineObserve(b *testing.B) {
	baseline := NewBaseline(0.001, 4)

	b.ReportAllocs()

	for b.Loop() {
		_ = baseline.Observe(0.01, 0.1)
	}
}
