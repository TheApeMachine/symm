package arithmetic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

func drive(
	op core.Primitive,
	mapping data.Map[string],
	inputs map[string]float64,
	output string,
) (float64, bool) {
	values := data.NewOutputMap()

	for key, value := range inputs {
		values.Values[key] = value
	}

	adapter := data.NewAdapter(nil, data.NewState(mapping, values))

	for range op.Next(data.NewValue(adapter)) {
	}

	value, ok := values.Values[output]
	return value, ok
}

func TestAddNext(t *testing.T) {
	Convey("Add speaks only augend, addend and sum", t, func() {
		op := NewAdd()
		mapping := data.NewMap(
			"augend", "left",
			"addend", "right",
			"sum", "result",
		)

		value, ok := drive(op, mapping, map[string]float64{"left": 2, "right": 3}, "result")
		So(ok, ShouldBeTrue)
		So(value, ShouldEqual, 5)

		value, ok = drive(op, mapping, map[string]float64{"left": -2, "right": 3}, "result")
		So(ok, ShouldBeTrue)
		So(value, ShouldEqual, 1)
	})
}
