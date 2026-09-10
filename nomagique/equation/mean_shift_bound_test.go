package equation

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestMeanShiftBound(t *testing.T) {
	Convey("The cut uses both subwindows' reciprocal sample support", t, func() {
		shift := MeanShift{Variance: 4, Observations: 6, RecentCount: 3, PriorCount: 3}
		So(shift.Bound(), ShouldAlmostEqual, math.Sqrt(4*math.Log(144)/3))
		shift.RecentCount, shift.PriorCount = 2, 4
		So(shift.Bound(), ShouldBeGreaterThan, math.Sqrt(4*math.Log(144)/3))
	})
}

func TestMeanShiftBoundNext(t *testing.T) {
	Convey("Named graph input preserves the typed cut", t, func() {
		op := NewMeanShiftBound()
		result, err := transport.Evaluate(op, transport.Values(MeanShift{
			Variance: 4, Observations: 6, RecentCount: 3, PriorCount: 3,
		}))
		So(err, ShouldBeNil)
		So(result, ShouldAlmostEqual, math.Sqrt(4*math.Log(144)/3))
	})
}

func BenchmarkMeanShiftBound(b *testing.B) {
	shift := MeanShift{Variance: 4, Observations: 6, RecentCount: 3, PriorCount: 3}
	b.ReportAllocs()

	for b.Loop() {
		shift.Bound()
	}
}
