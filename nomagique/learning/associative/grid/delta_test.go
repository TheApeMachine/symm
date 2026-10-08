package grid

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestDelta(t *testing.T) {
	Convey("Given a Delta primitive configured for 2 regions", t, func() {
		delta := NewDelta(2)

		Convey("Sequential steps compute levels and changes accurately", func() {
			out1 := tests.CollectSeq[float64](delta.Next(data.NewValue(10.0, 20.0).Next(nil)))
			So(delta.Error(), ShouldBeNil)
			// Region 1: ID=1, level=10, change=10-0=10
			So(out1[0], ShouldEqual, 10.0)
			So(out1[1], ShouldEqual, 10.0)
			So(out1[2], ShouldEqual, 1.0)
			// Region 2: ID=2, level=20, change=20-0=20
			So(out1[3], ShouldEqual, 20.0)
			So(out1[4], ShouldEqual, 20.0)
			So(out1[5], ShouldEqual, 2.0)

			out2 := tests.CollectSeq[float64](delta.Next(data.NewValue(12.0, 15.0).Next(nil)))
			So(delta.Error(), ShouldBeNil)
			// Region 1: ID=1, level=12, change=12-10=2
			So(out2[0], ShouldEqual, 12.0)
			So(out2[1], ShouldEqual, 2.0)
			// Region 2: ID=2, level=15, change=15-20=-5
			So(out2[3], ShouldEqual, 15.0)
			So(out2[4], ShouldEqual, -5.0)
		})

		Convey("Shape mismatch records ErrShape", func() {
			tests.CollectSeq[float64](delta.Next(data.NewValue(1.0).Next(nil)))
			So(errors.Is(delta.Error(), core.ErrShape), ShouldBeTrue)
		})
	})
}
