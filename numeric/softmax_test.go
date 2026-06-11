package numeric

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSoftmaxScores(t *testing.T) {
	Convey("Given raw logits", t, func() {
		probabilities, err := SoftmaxScores([]float64{1, 2, 3})

		sum := 0.0

		for _, probability := range probabilities {
			sum += probability
		}

		Convey("It should normalize to unity", func() {
			So(err, ShouldBeNil)
			So(sum, ShouldAlmostEqual, 1.0, 1e-9)
			So(ArgmaxIndex(probabilities), ShouldEqual, 2)
		})
	})

	Convey("Given non-finite logits", t, func() {
		_, err := SoftmaxScores([]float64{1, math.NaN(), 3})

		So(err, ShouldNotBeNil)

		_, err = SoftmaxScores([]float64{1, math.Inf(1), 3})

		So(err, ShouldNotBeNil)
	})
}

func BenchmarkSoftmaxScores(b *testing.B) {
	scores := []float64{0.6, 0.4, 0.7, 0.3}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = SoftmaxScores(scores)
	}
}
