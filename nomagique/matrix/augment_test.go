package matrix_test

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestAugmentNext(t *testing.T) {
	Convey("Given two matrices of matching row counts", t, func() {
		node := matrix.NewAugment()
		left := [][]float64{{1, 2}, {3, 4}}
		right := [][]float64{{5, 6, 7}, {8, 9, 10}}
		in := matrix.AugmentInput{Left: left, Right: right}
		seq := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&in))
		}
		out := tests.CollectSeq[[][]float64](node.Next(seq))
		So(node.Error(), ShouldBeNil)
		So(len(out), ShouldEqual, 1)
		So(out[0], ShouldResemble, [][]float64{{1, 2, 5, 6, 7}, {3, 4, 8, 9, 10}})
	})

	Convey("Mismatched row counts return ErrShape", t, func() {
		node := matrix.NewAugment()
		left := [][]float64{{1, 2}, {3, 4}}
		right := [][]float64{{5}}
		in := matrix.AugmentInput{Left: left, Right: right}
		seq := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&in))
		}
		tests.CollectSeq[[][]float64](node.Next(seq))
		So(errors.Is(node.Error(), core.ErrShape), ShouldBeTrue)
	})
}
