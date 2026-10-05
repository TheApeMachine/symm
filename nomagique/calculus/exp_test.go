package calculus

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestExp(t *testing.T) {
	Convey("Exp computes exponential function", t, func() {
		op := NewExp()
		state := data.NewState(data.NewMap("value", "value"))
		adapter := data.NewAdapter(nil, state)
		input := data.NewOutputMap()
		input.Values["value"] = 1.0

		for range adapter.Next(data.NewValue(input)) {
		}

		data.Read[*data.Adapter](op.Next(data.NewValue(adapter)))
		So(op.Error(), ShouldBeNil)
		So(op.output.Values["value"], ShouldAlmostEqual, math.E, 1e-9)
	})
}
