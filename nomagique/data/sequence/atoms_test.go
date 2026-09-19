package sequence_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestSequenceAtoms(t *testing.T) {
	Convey("Given sequence atoms", t, func() {
		Convey("Window", func() {
			win := sequence.NewWindow[int](types.Const(3))
			So(win(1), ShouldResemble, []int{1})
			So(win(2), ShouldResemble, []int{1, 2})
			So(win(3), ShouldResemble, []int{1, 2, 3})
			So(win(4), ShouldResemble, []int{2, 3, 4})
		})

		Convey("Tail", func() {
			tail := sequence.NewTail[int](types.Const(2))
			So(tail([]int{1, 2, 3, 4}), ShouldResemble, []int{3, 4})
		})

		Convey("At", func() {
			at := sequence.NewAt[string](types.Const(1))
			So(at([]string{"a", "b", "c"}), ShouldEqual, "b")
		})

		Convey("Values", func() {
			vals := sequence.NewValues(types.Const("x"), types.Const("y"))
			So(vals(nil), ShouldResemble, []string{"x", "y"})
		})

		Convey("Order", func() {
			order := sequence.NewOrder[int]()
			So(order([]int{5, 2, 8, 1}), ShouldResemble, []int{1, 2, 5, 8})
		})
	})
}
