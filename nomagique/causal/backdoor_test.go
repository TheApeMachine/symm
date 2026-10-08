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
			var results []float64

			for pointer := range node.Next(data.NewValue(level, 2.0, 3.0).Next(nil)) {
				results = append(results, *(*float64)(pointer))
			}

			So(node.Error(), ShouldBeNil)
			So(len(results), ShouldEqual, 2)
			So(results[0], ShouldAlmostEqual, 2+3*level)
			So(results[1], ShouldEqual, 1.0)
		}
	})
}
