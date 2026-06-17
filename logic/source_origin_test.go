package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSourceFromSignalOrigin(testingTB *testing.T) {
	Convey("Given dashboard signal registry names", testingTB, func() {
		Convey("It should map exhaust to exhaustion", func() {
			So(SourceFromSignalOrigin("exhaust"), ShouldEqual, SourceExhaustion)
		})

		Convey("It should preserve other signal names", func() {
			So(SourceFromSignalOrigin("hawkes"), ShouldEqual, SourceHawkes)
		})
	})
}
