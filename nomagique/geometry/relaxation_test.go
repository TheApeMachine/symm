package geometry

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRelaxationStep(t *testing.T) {
	Convey("The weak endpoint moves more toward a strong endpoint", t, func() {
		left, right := &Point{Authority: 1}, &Point{X: 4, Authority: 3}
		Relaxation{}.Step([]*Point{left, right}, []Edge{{Left: 0, Right: 1, Strength: 1}})
		So(left.X, ShouldBeGreaterThan, 4-right.X)
		So(right.X-left.X, ShouldAlmostEqual, 0.5)
	})
}

func BenchmarkRelaxationStep(b *testing.B) {
	points := []*Point{{Authority: 1}, {X: 4, Authority: 3}}
	edges := []Edge{{Left: 0, Right: 1, Strength: 1}}
	b.ReportAllocs()

	for index := 0; index < b.N; index++ {
		Relaxation{}.Step(points, edges)
	}
}
