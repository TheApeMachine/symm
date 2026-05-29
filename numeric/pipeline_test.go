package numeric

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/numeric/adaptive"
)

func TestLabelTapNext(t *testing.T) {
	Convey("Given a label tap over price moves", t, func() {
		classifier := mustClassifier()
		label := NewLabelTap(classifier)

		_, err := label.Next(0.8, 101, 100)

		Convey("It should pass confidence through and record a class code", func() {
			So(err, ShouldBeNil)
			So(label.ClassCode(), ShouldEqual, 2)
		})
	})
}

func TestScaleIndexNext(t *testing.T) {
	Convey("Given a scale-index stage", t, func() {
		stage := NewScaleIndex(1)

		out, err := stage.Next(2, 1, 3)

		Convey("It should multiply by the selected observation", func() {
			So(err, ShouldBeNil)
			So(out, ShouldAlmostEqual, 6, 1e-12)
		})

		Convey("It should return zero when the index is out of range", func() {
			out, err := stage.Next(2, 1)
			So(err, ShouldBeNil)
			So(out, ShouldEqual, 0)
		})
	})
}

func TestScoredClassCode(t *testing.T) {
	Convey("Given a scored pipeline", t, func() {
		scored := NewScored(mustClassifier(), adaptive.NewProduct())

		_, err := scored.Push(100, 101)

		Convey("It should expose the latest class code from the label tap", func() {
			So(err, ShouldBeNil)
			So(scored.ClassCode(), ShouldBeGreaterThanOrEqualTo, 0)
		})
	})
}

func BenchmarkScaleIndexNext(b *testing.B) {
	stage := NewScaleIndex(1)

	for b.Loop() {
		_, _ = stage.Next(2, 1, 3)
	}
}
