package calculus

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestMaximum(t *testing.T) {
	Convey("Maximum tracks the running maximum", t, func() {
		op := NewMaximum()
		state := data.NewState(data.NewMap("value", "value"))
		adapter := data.NewAdapter(nil, state)

		inputs := []float64{3.0, 5.0, 2.0, 8.0, 1.0}
		expected := []float64{3.0, 5.0, 5.0, 8.0, 8.0}

		for index, inputVal := range inputs {
			inputMap := data.NewOutputMap()
			inputMap.Values["value"] = inputVal
			for range adapter.Next(data.NewValue(inputMap)) {
			}
			data.Read[*data.Adapter](op.Next(data.NewValue(adapter)))
			So(op.Error(), ShouldBeNil)
			So(op.output.Values["value"], ShouldEqual, expected[index])
		}
	})
}
