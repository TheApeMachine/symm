package leadlag

import (
	"runtime"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
)

func TestPipelineObserve(t *testing.T) {
	Convey("Given independent ordered market pairs", t, func() {
		focal, other := newPipeline(), newPipeline()
		pair := core.To[map[string]core.Primitive](core.Record(map[string]any{
			"x": 5.0, "absolute_gain": 0.1, "correlation": 0.6,
		}))
		So(focal.Observe(pair, int64(time.Second)), ShouldBeNil)

		Convey("Other markets cannot train this pair's baselines or velocities", func() {
			pair["x"] = core.From(500.0)
			So(other.Observe(pair, int64(time.Second)), ShouldBeNil)
			pair["x"], pair["absolute_gain"], pair["correlation"] = core.From(7.0), core.From(-0.2), core.From(-0.5)
			So(focal.Observe(pair, int64(2*time.Second)), ShouldBeNil)
			So(focal.histories[lagHistory].Reading.Baseline, ShouldEqual, 5)
			So(focal.velocities[lagHistory].Reading.Rate, ShouldEqual, 2)
			So(focal.velocities[gainHistory].Reading.Rate, ShouldAlmostEqual, -0.3)
			So(other.histories[lagHistory].Reading.Baseline, ShouldEqual, 500)
			So(other.velocities[lagHistory].Reading.HasPrior, ShouldBeFalse)
		})

		Convey("A malformed pair cannot partially advance any history", func() {
			delete(pair, "correlation")
			So(focal.Observe(pair, int64(2*time.Second)), ShouldNotBeNil)
			So(focal.histories[lagHistory].Reading.Count, ShouldEqual, 1)
			So(focal.velocities[lagHistory].Reading.Through.At, ShouldEqual, time.Second)
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
	pair := core.To[map[string]core.Primitive](core.Record(map[string]any{
		"x": 5.0, "absolute_gain": 0.1, "correlation": 0.6,
	}))
	at := int64(0)
	b.ReportAllocs()
	for b.Loop() {
		at++
		if err := pipeline.Observe(pair, at); err != nil {
			b.Fatal(err)
		}
	}
}
