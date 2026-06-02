package numeric

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/numeric/adaptive"
)

func TestLabelTapNext(t *testing.T) {
	Convey("Given a label tap over price moves", t, func() {
		classifier := adaptive.NewClassifier(
			[]float64{0},
			[]float64{0, 1},
			[]string{"down", "up"},
		)
		label := NewLabelTap(classifier)

		Convey("It should classify upward moves without changing confidence", func() {
			out, err := label.Next(0.75, 100, 101)

			So(err, ShouldBeNil)
			So(out, ShouldEqual, 0.75)
			So(label.ClassCode(), ShouldEqual, 1)
		})

		Convey("It should reset class code", func() {
			_, _ = label.Next(0, 100, 99)
			So(label.Reset(), ShouldBeNil)
			So(label.ClassCode(), ShouldEqual, 0)
		})
	})
}

func BenchmarkLabelTapNext(b *testing.B) {
	classifier := adaptive.NewClassifier(
		[]float64{0},
		[]float64{0, 1},
		[]string{"down", "up"},
	)
	label := NewLabelTap(classifier)

	for b.Loop() {
		_, _ = label.Next(0, 100, 101)
	}
}
