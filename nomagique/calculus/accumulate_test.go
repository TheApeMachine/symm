package calculus

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestAccumulate(t *testing.T) {
	Convey("Given an Accumulate primitive starting at 0", t, func() {
		accumulate := NewAccumulate(0)

		Convey("Adding 3 and then 4 should yield 7", func() {
			accumulate.Write(types.NewInput(types.NewValue(3.0)))
			accumulate.Read()

			accumulate.Write(types.NewInput(types.NewValue(4.0)))
			out := accumulate.Read()
			So(out.Error(), ShouldBeBlank)
			So(out.Project().Read(), ShouldEqual, 7.0)
		})

		Convey("Close should succeed", func() {
			So(accumulate.Close(), ShouldBeNil)
		})
	})
}
