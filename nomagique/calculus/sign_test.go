package calculus

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestSign(t *testing.T) {
	Convey("Sign computes unit sign of arrival", t, func() {
		op := NewSign()
		state := data.NewState(data.NewMap("value", "value"))
		adapter := data.NewAdapter(nil, state)

		inputs := []float64{10.0, -5.0, 0.0}
		expected := []float64{1.0, -1.0, 0.0}

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
