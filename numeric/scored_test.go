package numeric

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/numeric/adaptive"
)

func TestNewScored(t *testing.T) {
	Convey("Given a scored pipeline", t, func() {
		classifier := adaptive.NewClassifier(
			[]float64{0},
			[]float64{0, 1},
			[]string{"down", "up"},
		)
		scored := NewScored(classifier, NewScaleIndex(1))

		Convey("It should push values and expose class code", func() {
			confidence, err := scored.Push(1, 2)

			So(err, ShouldBeNil)
			So(confidence, ShouldEqual, 2)
			So(scored.ClassCode(), ShouldEqual, 1)
		})
	})
}

func BenchmarkScoredPush(b *testing.B) {
	classifier := adaptive.NewClassifier(
		[]float64{0},
		[]float64{0, 1},
		[]string{"down", "up"},
	)
	scored := NewScored(classifier, NewScaleIndex(0))

	for b.Loop() {
		_, _ = scored.Push(1, 1.01)
	}
}
