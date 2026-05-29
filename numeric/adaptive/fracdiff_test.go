package adaptive

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFracDiffOrderZeroIsIdentity(t *testing.T) {
	Convey("Given a fractional differencer of order zero", t, func() {
		fracDiff := NewFracDiff(0, 4)

		var last float64
		var ok bool

		for _, value := range []float64{2, 3, 5, 7} {
			last, ok = fracDiff.Push(value)
		}

		Convey("Once warm it should return the input unchanged", func() {
			So(ok, ShouldBeTrue)
			So(last, ShouldAlmostEqual, 7, 1e-9)
		})
	})
}

func TestFracDiffOrderOneIsFirstDifference(t *testing.T) {
	Convey("Given a fractional differencer of order one", t, func() {
		fracDiff := NewFracDiff(1, 4)

		var last float64

		for _, value := range []float64{10, 12, 15, 19} {
			last, _ = fracDiff.Push(value)
		}

		Convey("Once warm it should equal the first difference", func() {
			So(last, ShouldAlmostEqual, 19-15, 1e-9)
		})
	})
}

func TestFracDiffWithholdsUntilWarm(t *testing.T) {
	Convey("Given a width-3 differencer", t, func() {
		fracDiff := NewFracDiff(0.4, 3)

		Convey("It should report not-warm until the window fills", func() {
			_, ok := fracDiff.Push(1)
			So(ok, ShouldBeFalse)

			_, ok = fracDiff.Push(2)
			So(ok, ShouldBeFalse)

			_, ok = fracDiff.Push(3)
			So(ok, ShouldBeTrue)
		})
	})
}

func TestFracDiffPreservesMemoryBeyondFirstDifference(t *testing.T) {
	Convey("Given a fractional order between zero and one", t, func() {
		fracDiff := NewFracDiff(0.4, 8)

		// Feed a steady upward drift. Integer differencing would map a constant slope to a
		// constant; fractional differencing keeps a memory tail, so the weights past lag one
		// are non-zero and the output reflects more than just the latest step.
		var value float64
		var out float64

		for index := 0; index < 8; index++ {
			value += 1
			out, _ = fracDiff.Push(value)
		}

		Convey("The differenced value should differ from a pure first difference of 1", func() {
			So(math.Abs(out-1) > 1e-6, ShouldBeTrue)
		})
	})
}
