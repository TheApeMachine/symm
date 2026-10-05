package causal_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/causal"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestLinearPredictionNext(t *testing.T) {
	Convey("LinearPrediction evaluates combination of intercept, weight and feature", t, func() {
		node := causal.NewLinearPrediction()

		state := data.NewState(data.NewMap(
			"intercept", "intercept",
			"weight", "weight",
			"feature", "feature",
		))
		adapter := data.NewAdapter(nil, state)
		input := data.NewOutputMap()
		input.Values["intercept"] = 2.0
		input.Values["weight"] = 3.0
		input.Values["feature"] = 4.0

		for range adapter.Next(data.NewValue(input)) {
		}

		data.Read[*data.Adapter](node.Next(data.NewValue(adapter)))
		So(node.Error(), ShouldBeNil)

		outputState := data.NewMap("prediction", "prediction")
		var result data.Map[float64]

		for pointer := range adapter.Next(data.NewValue(outputState)) {
			result = *(*data.Map[float64])(pointer)
		}

		So(result.Values["prediction"], ShouldEqual, 2.0+3.0*4.0)
	})
}
