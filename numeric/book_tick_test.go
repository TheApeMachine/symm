package numeric

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestInferBookTickSize(t *testing.T) {
	Convey("Given adjacent book prices", t, func() {
		bids := []float64{99.9, 99.8, 99.7}
		asks := []float64{100.0, 100.1, 100.2}

		Convey("It should infer the minimum positive spacing", func() {
			So(InferBookTickSize(bids, asks), ShouldAlmostEqual, 0.1, 1e-9)
		})
	})
}
