package leadlag

import (
	"runtime"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPipelineObserve(t *testing.T) {
	Convey("Given independent ordered market pairs", t, func() {
		focal, other := newPipeline(), newPipeline()
		pair := pairObservation{Lag: 5, AbsoluteGain: 0.1, Correlation: 0.6}
		focal.Observe(pair, int64(time.Second))

		Convey("Other markets cannot train this pair's baselines or velocities", func() {
			other.Observe(pairObservation{Lag: 500, AbsoluteGain: 0.1, Correlation: 0.6}, int64(time.Second))
			focal.Observe(pairObservation{Lag: 7, AbsoluteGain: -0.2, Correlation: -0.5}, int64(2*time.Second))
			So(focal.lag.Baseline, ShouldEqual, 5)
			So(focal.lagVel.Rate, ShouldEqual, 2)
			So(focal.gainVel.Rate, ShouldAlmostEqual, -0.3)
			So(other.lag.Baseline, ShouldEqual, 500)
			So(other.lagVel.HasPrior, ShouldBeFalse)
		})
	})
}

func BenchmarkNewPipeline(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		runtime.KeepAlive(newPipeline())
	}
}

func BenchmarkPipelineObserve(b *testing.B) {
	pipeline := newPipeline()
	pair := pairObservation{Lag: 5, AbsoluteGain: 0.1, Correlation: 0.6}
	at := int64(0)
	b.ReportAllocs()
	for b.Loop() {
		at++
		pipeline.Observe(pair, at)
	}
}
