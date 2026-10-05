package logic

import (
	"errors"
	"math"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestFiniteNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, bool]{
			Name: "finite",
			Seed: false,
			Factory: func() core.Primitive {
				return NewFinite()
			},
			Reference: func(_ bool, value float64) bool {
				return !math.IsNaN(value) && !math.IsInf(value, 0)
			},
		},
	)

	Convey("Given a Finite primitive", t, func() {
		op := NewFinite()

		Convey("When a nil pointer arrives, it records ErrShape", func() {
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[bool](op.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(op.Error(), core.ErrShape), ShouldBeTrue)
		})
	})
}
