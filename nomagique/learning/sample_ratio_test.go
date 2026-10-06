package learning_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning"
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

			got := data.Read[[3]float64](node.Next(data.NewValue([2]float64{predicted, predicted + residual})))

			So(node.Error(), ShouldBeNil)
			So(got[1], ShouldBeGreaterThanOrEqualTo, got[0])
		}
	})
}
