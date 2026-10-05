package statistic

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestEnergyNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name: "energy",
			Seed: 0.0,
			Factory: func() core.Primitive {
				return NewEnergy()
			},
			Reference: func(acc, val float64) float64 {
				return acc + val*val
			},
		},
	)

	Convey("Given an Energy primitive", t, func() {
		op := NewEnergy()

		Convey("When a nil pointer arrives, it records ErrShape", func() {
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[float64](op.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(op.Error(), core.ErrShape), ShouldBeTrue)
		})
	})
}
