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

func TestTransposeNext(t *testing.T) {
	Convey("Transpose changes addressing without mutating the source", t, func() {
		node := matrix.NewTranspose[string]()
		original := [][]string{{"a", "b", "c"}, {"d", "e", "f"}}
		want := [][]string{{"a", "d"}, {"b", "e"}, {"c", "f"}}
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&original))
		}
		out := tests.CollectSeq[[][]string](node.Next(in))
		So(node.Error(), ShouldBeNil)
		So(out[0], ShouldResemble, want)
		out[0][0][0] = "changed"
		So(original[0][0], ShouldEqual, "a")
	})

	Convey("Each arrival is transposed independently", t, func() {
		node := matrix.NewTranspose[string]()
		original := [][]string{{"a", "b", "c"}, {"d", "e", "f"}}
		second := [][]string{{"last"}}
		in := func(yield func(unsafe.Pointer) bool) {
			if !yield(unsafe.Pointer(&original)) {
				return
			}
			yield(unsafe.Pointer(&second))
		}
		out := tests.CollectSeq[[][]string](node.Next(in))
		So(out[0], ShouldResemble, [][]string{{"a", "d"}, {"b", "e"}, {"c", "f"}})
		So(out[1], ShouldResemble, [][]string{{"last"}})
	})

	Convey("Ragged rows are a shape error", t, func() {
		bad := matrix.NewTranspose[float64]()
		badInput := [][]float64{{1, 2}, {3}}
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&badInput))
		}
		tests.CollectSeq[[][]float64](bad.Next(in))
		So(errors.Is(bad.Error(), core.ErrShape), ShouldBeTrue)
	})
}
