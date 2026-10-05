package logic

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestNotNext(t *testing.T) {
	tests.Check(
		t, tests.Case[bool, bool]{
			Name: "not",
			Seed: false,
			Factory: func() core.Primitive {
				return NewNot()
			},
			Reference: func(_, value bool) bool {
				return !value
			},
			CustomVectors: [][]bool{
				{false},
				{true, false, true},
			},
		},
	)

	Convey("Given a Not primitive", t, func() {
		op := NewNot()

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
