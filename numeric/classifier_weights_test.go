package numeric

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestClassifierWeightsScores(t *testing.T) {
	Convey("Given classifier weights", t, func() {
		weights := DefaultClassifierWeights(2.0)
		scores := weights.Scores(2.0, 0.1, 1.5)

		Convey("It should return four logits", func() {
			So(len(scores), ShouldEqual, 4)
			So(scores[0], ShouldBeGreaterThan, scores[3])
		})
	})
}
