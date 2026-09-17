package geometry_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/geometry"
)

func TestCoordinateIdentity(t *testing.T) {
	Convey("A coordinate is its own typed identity", t, func() {
		coordinate := geometry.NewCoordinate(3, -2)
		var identifiable core.Identifiable[*geometry.Coordinate] = coordinate
		So(identifiable.Identity(), ShouldEqual, coordinate)
	})
}

func TestCoordinateIdentify(t *testing.T) {
	Convey("Identification updates the point while preserving its primitive identity", t, func() {
		coordinate := geometry.NewCoordinate(3, -2)
		originalErrors := coordinate.PrimitiveError
		for _, pair := range [][2]int{{0, 0}, {-5, 8}, {3, -2}} {
			So(coordinate.Identify(geometry.NewCoordinate(pair[0], pair[1])), ShouldEqual, coordinate)
			So(coordinate.Identity(), ShouldEqual, coordinate)
			So(coordinate.X, ShouldEqual, pair[0])
			So(coordinate.Y, ShouldEqual, pair[1])
			So(coordinate.PrimitiveError, ShouldEqual, originalErrors)
		}
	})
}

func TestCoordinateLess(t *testing.T) {
	Convey("Coordinates order lexicographically by X then Y", t, func() {
		points := []*geometry.Coordinate{
			geometry.NewCoordinate(-1, 7), geometry.NewCoordinate(0, -1),
			geometry.NewCoordinate(0, 0), geometry.NewCoordinate(0, 1), geometry.NewCoordinate(1, -7),
		}
		for leftIndex, left := range points {
			var ordered core.Ordered[*geometry.Coordinate] = left
			for rightIndex, right := range points {
				So(ordered.Less(right), ShouldEqual, leftIndex < rightIndex)
			}
			So(left.Less(geometry.NewCoordinate(left.X, left.Y)), ShouldBeFalse)
		}
	})
}

func TestCoordinateNext(t *testing.T) {
	Convey("Coordinate yields resident X and Y addresses in order", t, func() {
		coordinate := geometry.NewCoordinate(3, -2)
		run := coordinate.Next(nil)
		for _, pair := range [][2]int{{3, -2}, {0, 0}, {-5, 8}} {
			coordinate.Identify(geometry.NewCoordinate(pair[0], pair[1]))
			pointers := []unsafe.Pointer{}
			values := []int{}
			for output := range run {
				pointers = append(pointers, output)
				values = append(values, *(*int)(output))
			}
			So(pointers, ShouldResemble, []unsafe.Pointer{unsafe.Pointer(&coordinate.X), unsafe.Pointer(&coordinate.Y)})
			So(values, ShouldResemble, pair[:])
		}
		count := 0
		for range run {
			count++
			break
		}
		So(count, ShouldEqual, 1)
	})
}

func BenchmarkCoordinateNext(b *testing.B) {
	coordinate := geometry.NewCoordinate(3, -2)
	run := coordinate.Next(nil)
	b.ReportAllocs()
	for b.Loop() {
		for output := range run {
			if output == nil {
				b.Fatal("missing coordinate component")
			}
		}
	}
}
