package statistic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestJointNext(t *testing.T) {
	Convey("Joint scores a vector only against prior multivariate covariance", t, func() {
		op := NewJoint(2)
		mapping := data.NewMap(
			"coordinate:0", "left",
			"coordinate:1", "right",
			"snr", "snr",
			"maturity", "maturity",
			"support", "support",
		)

		samples := [][2]float64{
			{1, 0},
			{0, 1},
			{-1, 0},
			{0, -1},
			{1, 1},
		}

		for index, sample := range samples {
			values := data.NewOutputMap()
			values.Values["left"] = sample[0]
			values.Values["right"] = sample[1]
			adapter := data.NewAdapter(nil, data.NewState(mapping, values))

			for range op.Next(data.NewValue(adapter)) {
			}

			if index < 3 {
				_, defined := values.Values["snr"]
				So(defined, ShouldBeFalse)
				continue
			}

			_, defined := values.Values["snr"]
			So(defined, ShouldBeTrue)
			So(values.Values["support"], ShouldEqual, float64(index))
			So(values.Values["maturity"], ShouldEqual, 1-1/float64(index))
		}
	})
}
