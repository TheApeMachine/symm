package causal_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/causal"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestBackdoorNext(t *testing.T) {
	Convey("Interventional expectation follows the affine structural model", t, func() {
		node := causal.NewBackdoor(1e-15)

		for _, level := range []float64{1, 0, 2, -1} {
			state := data.NewState(data.NewMap(
				"level", "level",
				"baseline", "baseline",
				"effect", "effect",
			))
			adapter := data.NewAdapter(nil, state)
			input := data.NewOutputMap()
			input.Values["level"] = level
			input.Values["baseline"] = 2.0
			input.Values["effect"] = 3.0

			for range adapter.Next(data.NewValue(input)) {
			}

			data.Read[*data.Adapter](node.Next(data.NewValue(adapter)))
			So(node.Error(), ShouldBeNil)

			outputState := data.NewMap("expectation", "expectation", "defined", "defined")
			var result data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(outputState)) {
				result = *(*data.Map[float64])(pointer)
			}

			So(result.Values["defined"], ShouldEqual, 1.0)
			So(result.Values["expectation"], ShouldAlmostEqual, 2+3*level)
		}
	})
}
