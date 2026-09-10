package linear_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/equation/linear"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestSpectralRadius2Next(t *testing.T) {
	Convey("Spectral radius of elementary 2-by-2 matrices", t, func() {
		op := linear.NewSpectralRadius2()

		for _, c := range []struct{ a, b, c, d, r float64 }{
			{0, -1, 1, 0, 1},
			{-3, 0, 0, 2, 3},
			{1, 0, 0, 1, 1},
		} {
			out := tests.CollectSeq(op.Next(transport.Values(linear.Matrix2{A: c.a, B: c.b, C: c.c, D: c.d})))
			So(op.Error(), ShouldBeNil)
			So(out[0], ShouldEqual, c.r)
		}
	})
}
