package guard

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGenerateIndexWindowSlices(t *testing.T) {
	Convey("Given a measurement tape length", t, func() {
		windows := GenerateIndexWindows(100, 0.5, 0.2, 0.1)

		Convey("It should emit rolling train/test slices", func() {
			So(len(windows), ShouldBeGreaterThan, 0)
			So(windows[0].TrainEnd, ShouldEqual, windows[0].TestStart)
			So(windows[0].TestEnd, ShouldBeLessThanOrEqualTo, 100)
		})
	})

	Convey("Given invalid fractions", t, func() {
		So(GenerateIndexWindows(100, 0, 0.2, 0.1), ShouldBeNil)
	})
}
