package logic

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestPickNext(t *testing.T) {
	Convey("Pick selects candidates according to predicate", t, func() {
		Convey("Greater picks running maximum", func() {
			pick := NewPick(NewGreater())
			in := tests.SliceToSeq([]float64{3.0, 1.0, 5.0, 2.0})
			out := tests.CollectSeq[float64](pick.Next(in))

			So(out, ShouldResemble, []float64{3.0, 3.0, 5.0, 5.0})
			So(pick.Error(), ShouldBeNil)
		})

		Convey("Less picks running minimum", func() {
			pick := NewPick(NewLess())
			in := tests.SliceToSeq([]float64{3.0, 1.0, 5.0, 2.0})
			out := tests.CollectSeq[float64](pick.Next(in))

			So(out, ShouldResemble, []float64{3.0, 1.0, 1.0, 1.0})
			So(pick.Error(), ShouldBeNil)
		})

		Convey("records error on nil arrival", func() {
			pick := NewPick(NewGreater())
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[float64](pick.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(pick.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("records error when predicate fails", func() {
			errPredicate := errors.New("predicate broken")
			pick := NewPick(NewReject(errPredicate))
			in := tests.SliceToSeq([]float64{3.0, 1.0})
			out := tests.CollectSeq[float64](pick.Next(in))

			So(len(out), ShouldEqual, 1)
			So(errors.Is(pick.Error(), errPredicate), ShouldBeTrue)
		})

		Convey("handles early consumer termination", func() {
			pick := NewPick(NewGreater())
			in := tests.SliceToSeq([]float64{3.0, 1.0, 5.0, 2.0})
			count := 0

			for range pick.Next(in) {
				count++
				break
			}

			So(count, ShouldEqual, 1)
			So(pick.Error(), ShouldBeNil)
		})
	})
}
