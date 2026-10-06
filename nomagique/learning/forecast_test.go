package learning_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning"
)

func TestForecastNext(t *testing.T) {
	Convey("Forecast scale follows residual surprise and mix recurrences", t, func() {
		node := learning.NewForecast()

		for index := 0; index < 100; index++ {
			predicted := 10.0 + float64(index%5)
			residual := 0.0

			if index > 7 {
				residual = 0.4 * math.Sin(float64(index)*0.17)
			}

			got := data.Read[[6]float64](node.Next(data.NewValue([2]float64{predicted, predicted + residual})))

			So(node.Error(), ShouldBeNil)
			So(got[0], ShouldEqual, got[1])
			So(got[4], ShouldEqual, float64(index+1))
		}
	})
}
