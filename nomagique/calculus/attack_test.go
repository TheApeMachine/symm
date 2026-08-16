package calculus

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestAttack(t *testing.T) {
	Convey("Given an Attack primitive with base 10.0", t, func() {
		params := types.NewMap[string, types.Value[float64]]()
		params.Put("base", types.NewValue(10.0))
		params.Put("jump", types.NewValue(2.5))

		attack := NewAttack(types.NewInput(types.NewValue(params)))

		Convey("A jump of 2.5 should yield 12.5", func() {
			attack.Write(types.NewInput(types.NewValue(params)))
			out := attack.Read()
			So(out.Error(), ShouldBeBlank)

			res, ok := out.Project().Read().Get("result")
			So(ok, ShouldBeTrue)
			So(res.Read(), ShouldEqual, 12.5)
		})

		Convey("Close should succeed", func() {
			So(attack.Close(), ShouldBeNil)
		})
	})
}
