package calculus

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestErfc(t *testing.T) {
	Convey("Erfc computes complementary error function", t, func() {
		op := NewErfc()
		state := data.NewState(data.NewMap("value", "value"))
		adapter := data.NewAdapter(nil, state)
		input := data.NewOutputMap()
		input.Values["value"] = 0.5

		for range adapter.Next(data.NewValue(input)) {
		}

		data.Read[*data.Adapter](op.Next(data.NewValue(adapter)))
		So(op.Error(), ShouldBeNil)
		So(op.output.Values["value"], ShouldAlmostEqual, math.Erfc(0.5), 1e-9)
	})
}
