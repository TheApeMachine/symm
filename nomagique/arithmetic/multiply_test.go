package arithmetic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestMultiplyNext(t *testing.T) {
	Convey("Multiply speaks only multiplicand, multiplier and product", t, func() {
		op := NewMultiply()
		mapping := data.NewMap(
			"multiplicand", "left",
			"multiplier", "right",
			"product", "result",
		)

		value, ok := drive(op, mapping, map[string]float64{"left": 100, "right": 2}, "result")
		So(ok, ShouldBeTrue)
		So(value, ShouldEqual, 200)

		value, ok = drive(op, mapping, map[string]float64{"left": -3, "right": 4}, "result")
		So(ok, ShouldBeTrue)
		So(value, ShouldEqual, -12)
	})
}
