package equation

import (
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
	"testing"
)

func TestMeanShiftBound(t *testing.T) {
	Convey("The cut uses both subwindows' reciprocal sample support", t, func() {
		shift := MeanShift{Variance: 4, Observations: 6, RecentCount: 3, PriorCount: 3}
		// ln(4 * 6^2), with (1/3 + 1/3)/2 = 1/3.
		So(shift.Bound(), ShouldAlmostEqual, math.Sqrt(4*math.Log(144)/3))
		shift.RecentCount, shift.PriorCount = 2, 4
		So(shift.Bound(), ShouldBeGreaterThan, math.Sqrt(4*math.Log(144)/3))
	})
}

func TestMeanShiftBoundNext(t *testing.T) {
	Convey("Named graph input preserves the typed cut and rejects missing support", t, func() {
		graph := NewMeanShiftBound()
		result, err := transport.Evaluate[float64](graph, core.Record(map[string]any{
			"variance": 4.0, "observations": 6.0, "recent_count": 3.0, "prior_count": 3.0,
		}))
		So(err, ShouldBeNil)
		So(result, ShouldAlmostEqual, math.Sqrt(4*math.Log(144)/3))
		_, err = transport.Evaluate[float64](graph, core.Record(map[string]any{"variance": 4.0}))
		So(err, ShouldNotBeNil)
	})
}

func BenchmarkMeanShiftBound(b *testing.B) {
	shift := MeanShift{Variance: 4, Observations: 6, RecentCount: 3, PriorCount: 3}
	b.ReportAllocs()
	for b.Loop() {
		shift.Bound()
	}
}
