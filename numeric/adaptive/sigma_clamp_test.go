package adaptive

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSigmaClampNext(t *testing.T) {
	t.Parallel()

	Convey("Given a SigmaClamp", t, func() {
		clamp := NewSigmaClamp(3, 4, 0.5)

		Convey("It should pass through values during warmup", func() {
			var last float64

			for range 3 {
				var err error

				last, err = clamp.Next(10.0)
				So(err, ShouldBeNil)
			}

			So(last, ShouldEqual, 10)
		})

		Convey("It should clamp a sudden astronomical spike toward the band", func() {
			clamp2 := NewSigmaClamp(2, 4, 0.25)

			_, _ = clamp2.Next(1.0)
			_, _ = clamp2.Next(1.05)
			_, _ = clamp2.Next(0.95)
			_, _ = clamp2.Next(1.02)

			out, err := clamp2.Next(1e9)
			So(err, ShouldBeNil)
			So(out, ShouldBeLessThan, 1e9)
			So(math.IsInf(out, 0), ShouldBeFalse)
		})
	})
}

func BenchmarkSigmaClampNext(b *testing.B) {
	clamp := NewSigmaClamp(3, 8, 0.0625)

	var x float64 = 0.5

	for range 20 {
		x, _ = clamp.Next(x + 0.01)
	}

	b.ResetTimer()

	for idx := 0; idx < b.N; idx++ {
		x, _ = clamp.Next(float64(idx%7) * 0.13)
	}
}
