package logic

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestEqualNext(t *testing.T) {
	tests.Check(
		t, tests.Case[[2]float64, bool]{
			Name: "equal",
			Seed: false,
			Factory: func() core.Primitive {
				return NewEqual()
			},
			Reference: func(_ bool, pair [2]float64) bool {
				return pair[0] == pair[1]
			},
			CustomVectors: [][][2]float64{
				{{2, 2}},
				{{1, 2}, {3, 3}},
			},
		},
	)

	Convey("Given an Equal primitive", t, func() {
		op := NewEqual()

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
