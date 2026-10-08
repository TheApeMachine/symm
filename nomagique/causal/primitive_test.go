package causal_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/causal"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestCausalPrimitive(t *testing.T) {
	Convey("Causal Primitive pipeline processes backdoor expectation", t, func() {
		node := causal.NewPrimitive(4, 2)

		var results []float64

		for pointer := range node.Next(data.NewValue(2.0, 1.0, 3.0).Next(nil)) {
			results = append(results, *(*float64)(pointer))
		}

		So(node.Error(), ShouldBeNil)
		So(len(results), ShouldEqual, 2)
		So(results[0], ShouldAlmostEqual, 1.0+3.0*2.0)
		So(results[1], ShouldEqual, 1.0)
	})
}
