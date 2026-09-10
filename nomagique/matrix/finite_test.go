package matrix_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestFiniteNext(t *testing.T) {
	Convey("Every run checks its own rows including empty and invalid matrices", t, func() {
		node := matrix.NewFinite()

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
			valid, err := transport.Evaluate(node, transport.Values(fixture.rows))
			So(err, ShouldBeNil)
			So(valid, ShouldEqual, fixture.valid)
			So(node.Read(), ShouldEqual, fixture.valid)
		}
	})
}
