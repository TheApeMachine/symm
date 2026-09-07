package matrix_test

import (
	"errors"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
	"testing"
)

func TestFiniteNext(t *testing.T) {
	Convey("Given the matrix validity predicate", t, func() {
		node := matrix.NewFinite()

		Convey("Every run checks its own rows including empty and invalid matrices", func() {
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
				valid, err := transport.Evaluate[bool](node, core.From(fixture.rows))
				So(err, ShouldBeNil)
				So(valid, ShouldEqual, fixture.valid)
				So(node.Read(), ShouldEqual, fixture.valid)
			}
		})

		Convey("Wrong input types remain explicit failures", func() {
			_, err := transport.Evaluate[bool](node, core.From("invalid"))
			So(errors.Is(err, core.ErrWrongType), ShouldBeTrue)
		})
	})
}
