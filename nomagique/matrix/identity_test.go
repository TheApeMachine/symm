package matrix_test

import (
	"errors"
	"fmt"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestIdentityNext(t *testing.T) {
	Convey("Given dimensions supplied at evaluation time", t, func() {
		Convey("An empty identity stays empty and earlier matrices remain immutable", func() {
			node1 := matrix.NewIdentity()
			size1 := 2.0
			seq1 := func(yield func(unsafe.Pointer) bool) {
				yield(unsafe.Pointer(&size1))
			}
			first := tests.CollectSeq[[][]float64](node1.Next(seq1))
			So(node1.Error(), ShouldBeNil)
			So(first[0], ShouldResemble, [][]float64{{1, 0}, {0, 1}})

			node2 := matrix.NewIdentity()
			size2 := 0.0
			seq2 := func(yield func(unsafe.Pointer) bool) {
				yield(unsafe.Pointer(&size2))
			}
			empty := tests.CollectSeq[[][]float64](node2.Next(seq2))
			So(node2.Error(), ShouldBeNil)
			So(empty[0], ShouldBeEmpty)
			So(first[0], ShouldResemble, [][]float64{{1, 0}, {0, 1}})
		})

		for _, size := range []float64{-1, 1.5} {
			Convey(fmt.Sprintf("Invalid dimension %g is rejected", size), func() {
				node := matrix.NewIdentity()
				s := size
				seq := func(yield func(unsafe.Pointer) bool) {
					yield(unsafe.Pointer(&s))
				}
				tests.CollectSeq[[][]float64](node.Next(seq))
				So(errors.Is(node.Error(), core.ErrShape), ShouldBeTrue)
			})
		}
	})
}
