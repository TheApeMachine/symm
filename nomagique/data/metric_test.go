package data

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDeformation(t *testing.T) {
	Convey("Given metric deformation calculation", t, func() {
		Convey("Direct required transitions", func() {
			So(Deformation(0, 0), ShouldEqual, 0)
			So(Deformation(0, 10), ShouldEqual, 1)
			So(Deformation(0, -10), ShouldEqual, -1)
			So(Deformation(10, 10), ShouldEqual, 0)
			So(Deformation(1, 2), ShouldAlmostEqual, Deformation(100, 200), 1e-15)
			So(Deformation(1, 2), ShouldAlmostEqual, 1.0/3.0, 1e-15)
			So(Deformation(2, 1), ShouldBeLessThan, 0)
			So(Deformation(2, 1), ShouldAlmostEqual, -1.0/3.0, 1e-15)
			So(Deformation(-2, -1), ShouldBeGreaterThan, 0)
			So(Deformation(-2, -1), ShouldAlmostEqual, 1.0/3.0, 1e-15)
			So(Deformation(-1, -2), ShouldBeLessThan, 0)
			So(Deformation(-1, -2), ShouldAlmostEqual, -1.0/3.0, 1e-15)
			So(Deformation(1, -1), ShouldEqual, 0)
			So(Deformation(-1, 1), ShouldEqual, 0)
		})

		Convey("Results are strictly bounded to [-1, 1]", func() {
			testPairs := [][2]float64{
				{0, 0}, {0, 10}, {0, -10}, {10, 10}, {1, 2}, {100, 200},
				{2, 1}, {-2, -1}, {-1, -2}, {1, -1}, {-1, 1},
				{1e9, 0}, {0, -1e9}, {-100, 50}, {50, -100},
			}

			for _, pair := range testPairs {
				result := Deformation(pair[0], pair[1])
				So(result, ShouldBeGreaterThanOrEqualTo, -1.0)
				So(result, ShouldBeLessThanOrEqualTo, 1.0)
			}
		})
	})
}
