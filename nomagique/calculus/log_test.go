package calculus

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

func driveAdapter(
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

func TestLogNext(t *testing.T) {
	Convey("Log speaks only argument and logarithm", t, func() {
		mapping := data.NewMap(
			"argument", "input",
			"logarithm", "output",
		)

		value, ok := driveAdapter(NewLog(), mapping, map[string]float64{"input": math.E}, "output")
		So(ok, ShouldBeTrue)
		So(value, ShouldAlmostEqual, 1)

		_, ok = driveAdapter(NewLog(), mapping, map[string]float64{"input": 0}, "output")
		So(ok, ShouldBeFalse)
	})
}
