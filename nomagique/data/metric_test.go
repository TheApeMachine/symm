package data

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDeformation(t *testing.T) {
	Convey("Given a metric pushing on its container", t, func() {
		Convey("No change is rest, including all-zero", func() {
			So(Deformation(75000, 75000), ShouldEqual, 0)
			So(Deformation(0, 0), ShouldEqual, 0)
		})

		Convey("Direction is kept and a rise mirrors a fall", func() {
			So(Deformation(75000, 76000), ShouldAlmostEqual, 1000.0/151000.0, 1e-15)
			So(Deformation(76000, 75000), ShouldAlmostEqual, -1000.0/151000.0, 1e-15)
		})

		Convey("Units drop out: equal relative moves deform alike", func() {
			So(Deformation(1, 2), ShouldAlmostEqual, 1.0/3.0, 1e-15)
			So(Deformation(100000, 200000), ShouldAlmostEqual, 1.0/3.0, 1e-15)
			So(Deformation(0.000003, 0.000006), ShouldAlmostEqual, 1.0/3.0, 1e-15)
		})

		Convey("A first value pushes a container at rest to a full swing", func() {
			So(Deformation(0, 75000), ShouldEqual, 1)
			So(Deformation(0, -0.5), ShouldEqual, -1)
		})

		Convey("A flip of equal magnitude is a full swing", func() {
			So(Deformation(0.5, -0.5), ShouldEqual, -1)
			So(Deformation(-0.5, 0.5), ShouldEqual, 1)
		})

		Convey("Moving toward zero is a push toward the metric's own sign change", func() {
			So(Deformation(-0.002, -0.001), ShouldAlmostEqual, 1.0/3.0, 1e-15)
		})
	})
}
