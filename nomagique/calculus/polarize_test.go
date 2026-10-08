package calculus_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestPolarize(t *testing.T) {
	Convey("Polarize splits signed values into nonnegative components", t, func() {
		op := calculus.NewPolarize()
		in := func(yield func(unsafe.Pointer) bool) {
			input1 := calculus.PolarizeInput{Value: 2.0, Scale: 2.0}
			input2 := calculus.PolarizeInput{Value: -2.0, Scale: 2.0}
			yield(unsafe.Pointer(&input1))
			yield(unsafe.Pointer(&input2))
		}
		out := tests.CollectSeq[calculus.PolarizeResult](op.Next(in))
		So(len(out), ShouldEqual, 2)
		So(out[0].Alpha, ShouldEqual, 2.0)
		So(out[0].Beta, ShouldEqual, 0.0)
		So(out[0].AlphaNormalized, ShouldEqual, 0.5)
		So(out[0].Value, ShouldEqual, 0.5)

		So(out[1].Alpha, ShouldEqual, 0.0)
		So(out[1].Beta, ShouldEqual, 2.0)
		So(out[1].BetaNormalized, ShouldEqual, 0.5)
		So(out[1].Value, ShouldEqual, -0.5)
	})
}
