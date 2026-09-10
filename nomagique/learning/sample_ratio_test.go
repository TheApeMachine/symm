package learning_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestSampleRatioNext(t *testing.T) {
	Convey("Sample ratio is bounded by the observed residual-range ceiling", t, func() {
		node := learning.NewSampleRatio()

		for index := 0; index < 20; index++ {
			predicted := 10.0 + float64(index%5)
			residual := 0.0

			if index > 7 {
				residual = 0.4 * math.Sin(float64(index)*0.17)
			}

			got, err := transport.Evaluate(node, transport.Values(learning.Pair{
				Predicted: predicted,
				Actual:    predicted + residual,
			}))
			So(err, ShouldBeNil)
			So(got.PeakRatio, ShouldBeGreaterThanOrEqualTo, got.Value)
		}
	})
}
