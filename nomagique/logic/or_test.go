package logic

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestOrNext(t *testing.T) {
	tests.Check(
		t, tests.Case[bool, bool]{
			Name: "or",
			Seed: false,
			Factory: func() core.Primitive {
				return NewOr(false)
			},
			Reference: func(held, value bool) bool {
				return held || value
			},
			CustomVectors: [][]bool{
				{false, true},
				{false, false, true},
				{true},
			},
		},
	)

	Convey("Given an Or primitive", t, func() {
		op := NewOr(false)

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
