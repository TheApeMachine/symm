package causal_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/causal"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestStumpNext(t *testing.T) {
	Convey("The stump strategy standardizes over the evidence", t, func() {
		rows := [][]float64{
			{0, 0, 1},
			{1, 0, 2},
			{0, 1, 4},
			{1, 1, 5},
			{2, 0, 3},
			{2, 1, 6},
		}
		node := causal.NewStump(rows, 2, 1, []int{0})
		So(node.Error(), ShouldBeNil)

		state := data.NewState(data.NewMap("level", "level"))
		adapter := data.NewAdapter(nil, state)
		input := data.NewOutputMap()
		input.Values["level"] = 1.0

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
		So(result.Values["expectation"], ShouldAlmostEqual, 2+3*1)
	})
}
