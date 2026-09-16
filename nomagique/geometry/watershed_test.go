package geometry

import (
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestWatershedStep(t *testing.T) {
	Convey("Two peaks meet at their weak cells", t, func() {
		points := []*Point{{X: 0, Authority: 1}, {X: 1, Authority: 0.25}, {X: 2, Authority: 0.25}, {X: 3, Authority: 1}}
		edges := []Edge{{Left: 0, Right: 1, Strength: 1}, {Left: 0, Right: 2, Strength: 1}, {Left: 1, Right: 2, Strength: 1},
			{Left: 0, Right: 3, Strength: 1}, {Left: 1, Right: 3, Strength: 1}, {Left: 2, Right: 3, Strength: 1}}
		Watershed{}.Step(points, edges)
		So(points[0].Basin, ShouldEqual, 0)
		So(points[1].Basin, ShouldEqual, 0)
		So(points[2].Basin, ShouldEqual, 3)
		So(points[3].Basin, ShouldEqual, 3)
	})
}

func BenchmarkWatershedStep(b *testing.B) {
	points := []*Point{{Authority: 1}, {X: 1, Authority: 0.25}, {X: 2, Authority: 1}}
	edges := []Edge{{Left: 0, Right: 1, Strength: 1}, {Left: 0, Right: 2, Strength: 1}, {Left: 1, Right: 2, Strength: 1}}
	b.ReportAllocs()

	for index := 0; index < b.N; index++ {
		Watershed{}.Step(points, edges)
	}
}
