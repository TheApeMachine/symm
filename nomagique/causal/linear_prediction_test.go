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

		var results []float64

		for pointer := range node.Next(data.NewValue(2.0, 3.0, 4.0).Next(nil)) {
			results = append(results, *(*float64)(pointer))
		}

		So(node.Error(), ShouldBeNil)
		So(len(results), ShouldEqual, 1)
		So(results[0], ShouldEqual, 2.0+3.0*4.0)
	})
}
