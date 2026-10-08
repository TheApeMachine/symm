package learning_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning"
)

func TestTrustWeightNext(t *testing.T) {
	Convey("Trust updates only after a positive residual span exists", t, func() {
		node := learning.NewTrustWeight()

		for index := 0; index < 20; index++ {
			predicted := 10.0 + float64(index%5)
			residual := 0.0

			if index > 7 {
				residual = 0.4 * math.Sin(float64(index)*0.17)
			}

			got := data.Read[[4]float64](node.Next(data.NewValue([2]float64{predicted, predicted + residual}).Next(nil)))

			So(node.Error(), ShouldBeNil)
			So(got[0], ShouldEqual, got[1])
			So(got[3], ShouldBeGreaterThan, 0)
		}
	})
}
