package logic

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestGateNext(t *testing.T) {
	Convey("Gate routes each arrival through pass or fail based on predicate", t, func() {
		Convey("routes to pass when predicate is true and fail when false", func() {
			gate := NewGate(NewAnd(true), NewNot(), NewNot())
			in := tests.SliceToSeq([]bool{true, false})
			out := tests.CollectSeq[bool](gate.Next(in))

			So(out, ShouldResemble, []bool{false, true})
			So(gate.Error(), ShouldBeNil)
		})

		Convey("routes with nil fail branch acting as a filter", func() {
			gate := NewGate(NewAnd(true), NewNot(), nil)
			in := tests.SliceToSeq([]bool{true, false})
			out := tests.CollectSeq[bool](gate.Next(in))

			So(out, ShouldResemble, []bool{false})
			So(gate.Error(), ShouldBeNil)
		})

		Convey("records error on nil arrival", func() {
			gate := NewGate(NewAnd(true), NewNot(), NewNot())
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[bool](gate.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(gate.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("records error when predicate fails", func() {
			errPredicate := errors.New("predicate broken")
			gate := NewGate(NewReject(errPredicate), NewNot(), NewNot())
			in := tests.SliceToSeq([]bool{true})
			out := tests.CollectSeq[bool](gate.Next(in))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(gate.Error(), errPredicate), ShouldBeTrue)
		})

		Convey("records error when branch fails", func() {
			errBranch := errors.New("branch broken")
			gate := NewGate(NewAnd(true), NewReject(errBranch), nil)
			in := tests.SliceToSeq([]bool{true})
			out := tests.CollectSeq[bool](gate.Next(in))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(gate.Error(), errBranch), ShouldBeTrue)
		})

		Convey("handles early consumer termination", func() {
			gate := NewGate(NewAnd(true), NewNot(), NewNot())
			in := tests.SliceToSeq([]bool{true, true, true})
			count := 0

			for range gate.Next(in) {
				count++
				break
			}

			So(count, ShouldEqual, 1)
			So(gate.Error(), ShouldBeNil)
		})
	})
}
