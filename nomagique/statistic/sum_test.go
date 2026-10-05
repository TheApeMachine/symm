package statistic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestSumNext(t *testing.T) {
	Convey("Sum owns one running native value", t, func() {
		op := NewSum()
		mapping := data.NewMap(
			"value", "input",
			"sum", "output",
		)

		total := 0.0

		for _, input := range []float64{1, 2, -0.5, 4} {
			values := data.NewOutputMap()
			values.Values["input"] = input
			adapter := data.NewAdapter(nil, data.NewState(mapping, values))

			for range op.Next(data.NewValue(adapter)) {
			}

			total += input
			So(values.Values["output"], ShouldEqual, total)
		}
	})
}
