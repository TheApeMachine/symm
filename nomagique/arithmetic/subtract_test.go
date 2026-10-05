package arithmetic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestSubtractNext(t *testing.T) {
	Convey("Subtract speaks only minuend, subtrahend and difference", t, func() {
		op := NewSubtract()
		mapping := data.NewMap(
			"minuend", "left",
			"subtrahend", "right",
			"difference", "result",
		)

		value, ok := drive(op, mapping, map[string]float64{"left": 5, "right": 3}, "result")
		So(ok, ShouldBeTrue)
		So(value, ShouldEqual, 2)

		value, ok = drive(op, mapping, map[string]float64{"left": 3, "right": 5}, "result")
		So(ok, ShouldBeTrue)
		So(value, ShouldEqual, -2)
	})
}
