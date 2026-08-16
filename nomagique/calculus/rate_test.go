package calculus

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestRate(t *testing.T) {
	Convey("Given a Rate primitive with count 2 and duration 4.0", t, func() {
		params := types.NewMap[string, types.Value[float64]]()
		params.Put("count", types.NewValue(2.0))
		params.Put("duration", types.NewValue(4.0))

		rate := NewRate(types.NewInput(types.NewValue(params)))

		Convey("An event count of 2 over 4.0s should yield rate 0.5", func() {
			rate.Write(types.NewInput(types.NewValue(params)))
			out := rate.Read()
			So(out.Error(), ShouldBeBlank)

			res, ok := out.Project().Read().Get("rate")
			So(ok, ShouldBeTrue)
			So(res.Read(), ShouldEqual, 0.5)
		})

		Convey("Close should succeed", func() {
			So(rate.Close(), ShouldBeNil)
		})
	})
}
