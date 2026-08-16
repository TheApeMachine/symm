package temporal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestClock(t *testing.T) {
	Convey("Given a clock with age 1.0 and span 2.0", t, func() {
		params := types.NewMap[string, types.Value[float64]]()
		params.Put("age", types.NewValue(1.0))
		params.Put("span", types.NewValue(2.0))

		clock := NewClock(types.NewInput(types.NewValue(params)))

		Convey("Read should emit progress 0.5", func() {
			clock.Write(types.NewInput(types.NewValue(params)))
			out := clock.Read()
			So(out.Error(), ShouldBeBlank)

			prog, ok := out.Project().Read().Get("progress")
			So(ok, ShouldBeTrue)
			So(prog.Read(), ShouldEqual, 0.5)
		})

		Convey("Close should reset state", func() {
			So(clock.Close(), ShouldBeNil)
		})
	})
}
