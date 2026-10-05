package calculus

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestMinimum(t *testing.T) {
	Convey("Minimum tracks the running minimum", t, func() {
		op := NewMinimum()
		state := data.NewState(data.NewMap("value", "value"))
		adapter := data.NewAdapter(nil, state)

		inputs := []float64{5.0, 3.0, 8.0, 1.0, 4.0}
		expected := []float64{5.0, 3.0, 3.0, 1.0, 1.0}

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
