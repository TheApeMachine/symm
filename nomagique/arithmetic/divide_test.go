package arithmetic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestDivideNext(t *testing.T) {
	Convey("Divide speaks only dividend, divisor and quotient", t, func() {
		op := NewDivide()
		mapping := data.NewMap(
			"dividend", "left",
			"divisor", "right",
			"quotient", "result",
		)

		value, ok := drive(op, mapping, map[string]float64{"left": 300, "right": 2}, "result")
		So(ok, ShouldBeTrue)
		So(value, ShouldEqual, 150)

		value, ok = drive(op, mapping, map[string]float64{"left": -6, "right": 4}, "result")
		So(ok, ShouldBeTrue)
		So(value, ShouldEqual, -1.5)

		_, ok = drive(op, mapping, map[string]float64{"left": 1, "right": 0}, "result")
		So(ok, ShouldBeFalse)
		So(op.Error(), ShouldBeNil)
	})
}
