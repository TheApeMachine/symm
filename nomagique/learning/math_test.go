package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAbsExact(testingTB *testing.T) {
	Convey("Given signed values", testingTB, func() {
		Convey("It should return exact magnitudes", func() {
			So(absExact(-4.25), ShouldEqual, 4.25)
			So(absExact(0), ShouldEqual, 0)
		})
	})
}
