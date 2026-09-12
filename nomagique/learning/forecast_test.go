package learning_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/transport"
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

			gotEval := transport.NewEvaluate(node)
			var got learning.ForecastReading

			for out := range gotEval.Next(transport.NewValues(learning.Pair{
				Predicted: predicted,
				Actual:    predicted + residual,
			}).Next(nil)) {
				got = *(*learning.ForecastReading)(out)
			}

			err := gotEval.Error()
			So(err, ShouldBeNil)
			So(got.Value, ShouldEqual, got.Scale)
			So(got.Count, ShouldEqual, float64(index+1))
		}
	})
}
