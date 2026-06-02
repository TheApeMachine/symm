package numeric

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/numeric/adaptive"
)

func TestClassedUnitMargins(t *testing.T) {
	Convey("Given the pumpdump ignition pipeline", t, func() {
		pipe := NewClassed(
			adaptive.NewClassifier(
				[]float64{-0.10, 0.50, 2.00},
				[]float64{0, 1, 2, 3},
				[]string{"faded_exhaustion", "organic_trend", "coiled_compression", "vertical_ignition"},
			),
			NewProjectScalar(func(_ float64, values []float64) float64 {
				return (values[0] - 1) * (1 + values[1])
			}),
			adaptive.NewEMA(0),
			adaptive.NewSigmaClamp(3, 8, 0.0625),
		)

		Convey("It should keep clarity and standout on the unit interval after an extreme push", func() {
			for range 24 {
				_, err := pipe.Push(1_000_000, 50_000)
				So(err, ShouldBeNil)
			}

			clarity := pipe.Confidence()
			standout := pipe.Standout()

			So(clarity, ShouldBeGreaterThanOrEqualTo, 0)
			So(clarity, ShouldBeLessThanOrEqualTo, 1)
			So(standout, ShouldBeGreaterThanOrEqualTo, 0)
			So(standout, ShouldBeLessThanOrEqualTo, 1)
		})
	})
}
