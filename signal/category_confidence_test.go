package signal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCategoryConfidence(t *testing.T) {
	Convey("Given decisive category scores", t, func() {
		confidence, err := CategoryConfidence([]float64{0, 0.8, 0.2, 0}, 1)

		Convey("It should preserve linear share when softmax dilutes", func() {
			So(err, ShouldBeNil)
			So(confidence, ShouldAlmostEqual, 0.8, 1e-9)
		})
	})
}
