package numeric

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAssertFinite(t *testing.T) {
	Convey("Given finite values", t, func() {
		Convey("It should accept them", func() {
			So(AssertFinite("value", 1.5), ShouldBeNil)
		})
	})

	Convey("Given non-finite values", t, func() {
		Convey("It should reject NaN", func() {
			So(AssertFinite("value", math.NaN()), ShouldNotBeNil)
		})

		Convey("It should reject positive infinity", func() {
			So(AssertFinite("value", math.Inf(1)), ShouldNotBeNil)
		})
	})
}

func TestAssertFiniteScores(t *testing.T) {
	Convey("Given finite scores", t, func() {
		err := AssertFiniteScores("softmax", []float64{1, 2, 3})

		Convey("It should accept them", func() {
			So(err, ShouldBeNil)
		})
	})

	Convey("Given a non-finite score", t, func() {
		err := AssertFiniteScores("softmax", []float64{1, math.NaN()})

		Convey("It should reject the slice", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkAssertFiniteScores(b *testing.B) {
	scores := []float64{0.1, 0.2, 0.3, 0.4}

	b.ResetTimer()

	for b.Loop() {
		if err := AssertFiniteScores("bench", scores); err != nil {
			b.Fatal(err)
		}
	}
}
