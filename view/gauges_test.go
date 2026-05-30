package view

import (
	"testing"

	"github.com/theapemachine/symm/market/perspectives"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGaugesFrame(t *testing.T) {
	Convey("Given a gauge feed", t, func() {
		gauges := &Gauges{}

		Convey("It should build a source/confidence frame from a known source", func() {
			frame, ok := gauges.frame(perspectives.Measurement{
				Source: perspectives.SourceFluid,
				SNR:    1.5,
			})

			So(ok, ShouldBeTrue)
			So(frame["source"], ShouldEqual, "fluid")
			So(frame["confidence"], ShouldEqual, 1.5)
		})

		Convey("It should skip a source with no dashboard name", func() {
			_, ok := gauges.frame(perspectives.Measurement{Source: perspectives.SourceNone})
			So(ok, ShouldBeFalse)
		})
	})
}
