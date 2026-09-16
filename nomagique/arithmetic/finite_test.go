package arithmetic_test

import (
	"math"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	arithmetic "github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestMatrixFiniteNext(t *testing.T) {
	Convey("Every run checks its own rows including empty and invalid matrices", t, func() {
		for _, fixture := range []struct {
			rows  [][]float64
			valid bool
		}{
			{[][]float64{{1, -2}, {3, 4}}, true},
			{[][]float64{{math.NaN()}}, false},
			{[][]float64{{math.Inf(-1)}}, false},
			{[][]float64{}, true},
			{[][]float64{{0}}, true},
		} {
			node := arithmetic.NewMatrixFinite()
			seq := func(yield func(unsafe.Pointer) bool) {
				yield(unsafe.Pointer(&fixture.rows))
			}
			out := tests.CollectSeq[bool](node.Next(seq))
			So(node.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			So(out[0], ShouldEqual, fixture.valid)
		}
	})
}
