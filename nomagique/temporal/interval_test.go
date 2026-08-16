package temporal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestInterval(t *testing.T) {
	Convey("Given an Interval primitive", t, func() {
		params := types.NewMap[string, types.Value[float64]]()
		params.Put("timestamp", types.NewValue(100.0))

		interval := NewInterval(types.NewInput(types.NewValue(params)))

		Convey("First timestamp should produce 0 delta", func() {
			interval.Write(types.NewInput(types.NewValue(params)))
			out := interval.Read()
			So(out.Error(), ShouldBeBlank)

			delta, ok := out.Project().Read().Get("delta")
			So(ok, ShouldBeTrue)
			So(delta.Read(), ShouldEqual, 0)
		})

		Convey("Second timestamp should produce positive delta", func() {
			interval.Write(types.NewInput(types.NewValue(params)))
			interval.Read()

			params2 := interval.Project().Read()
			params2.Put("timestamp", types.NewValue(100.5))
			interval.Write(types.NewInput(types.NewValue(params2)))
			out := interval.Read()
			So(out.Error(), ShouldBeBlank)

			delta, ok := out.Project().Read().Get("delta")
			So(ok, ShouldBeTrue)
			So(delta.Read(), ShouldEqual, 0.5)
		})

		Convey("Close should succeed", func() {
			So(interval.Close(), ShouldBeNil)
		})
	})
}
