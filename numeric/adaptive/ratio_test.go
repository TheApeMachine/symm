package adaptive

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewRatio(t *testing.T) {
	t.Parallel()

	Convey("Given NewRatio", t, func() {
		r := NewRatio(0)

		Convey("It should attach an internal EMA", func() {
			So(r.smoother, ShouldNotBeNil)
		})
	})
}

func TestRatioNext(t *testing.T) {
	t.Parallel()

	Convey("Given a Ratio", t, func() {
		r := NewRatio(0)

		Convey("It should require exactly two operands", func() {
			_, err := r.Next(0, 1)

			So(err, ShouldNotBeNil)

			_, err = r.Next(0, 1, 2, 3)

			So(err, ShouldNotBeNil)
		})

		Convey("It should reject a zero denominator", func() {
			_, err := r.Next(0, 1, 0)

			So(err, ShouldNotBeNil)
		})

		Convey("It should smooth valid ratios", func() {
			out, err := r.Next(0, 10, 2)

			So(err, ShouldBeNil)
			So(out, ShouldAlmostEqual, 5, 1e-6)
		})
	})
}

func TestRatioReset(t *testing.T) {
	t.Parallel()

	Convey("Given a Ratio after updates", t, func() {
		r := NewRatio(0)

		_, _ = r.Next(0, 4, 2)

		So(r.Reset(), ShouldBeNil)

		So(math.IsNaN(r.raw), ShouldBeTrue)

		again, err := r.Next(0, 8, 4)

		So(err, ShouldBeNil)
		So(again, ShouldAlmostEqual, 2, 1e-6)
	})
}

func BenchmarkRatioNext(b *testing.B) {
	r := NewRatio(0)

	var v float64

	var err error

	for idx := 0; idx < b.N; idx++ {
		den := float64((idx % 5) + 1)

		v, err = r.Next(v, float64(idx%19), den)

		if err != nil {
			b.Fatal(err)
		}
	}

	_ = v
}
