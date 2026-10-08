package causal_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/causal"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestLinearFitNext(t *testing.T) {
	Convey("LinearFit estimates parameters from stream", t, func() {
		node := causal.NewLinearFit(1e-15)

		var results []float64

		for pointer := range node.Next(data.NewValue(5.0, 1.0).Next(nil)) {
			results = append(results, *(*float64)(pointer))
		}

		So(node.Error(), ShouldBeNil)
		So(len(results), ShouldEqual, 3)
	})
}
