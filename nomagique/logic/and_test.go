package logic

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestAndNext(t *testing.T) {
	tests.Check(
		t, tests.Case[bool, bool]{
			Name: "and",
			Seed: true,
			Factory: func() core.Primitive {
				return NewAnd(true)
			},
			Reference: func(held, value bool) bool {
				return held && value
			},
			CustomVectors: [][]bool{
				{true, false},
				{true, true, true},
				{false},
			},
		},
	)

	Convey("Given an And primitive", t, func() {
		op := NewAnd(true)

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
