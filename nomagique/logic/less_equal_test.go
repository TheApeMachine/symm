package logic

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestLessEqualNext(t *testing.T) {
	tests.Check(
		t, tests.Case[[2]float64, bool]{
			Name: "less-equal",
			Seed: false,
			Factory: func() core.Primitive {
				return NewLessEqual()
			},
			Reference: func(_ bool, pair [2]float64) bool {
				return pair[0] <= pair[1]
			},
			CustomVectors: [][][2]float64{
				{{2, 2}},
				{{1, 2}, {3, 1}},
			},
		},
	)

	Convey("Given a LessEqual primitive", t, func() {
		op := NewLessEqual()

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
