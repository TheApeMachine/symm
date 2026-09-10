package matrix_test

import (
	"errors"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestIdentityNext(t *testing.T) {
	Convey("Given dimensions supplied at evaluation time", t, func() {
		Convey("An empty identity stays empty and earlier matrices remain immutable", func() {
			node := matrix.NewIdentity()
			first, err := transport.Evaluate(node, transport.Values(2.0))
			So(err, ShouldBeNil)
			So(first, ShouldResemble, [][]float64{{1, 0}, {0, 1}})
			empty, err := transport.Evaluate(node, transport.Values(0.0))
			So(err, ShouldBeNil)
			So(empty, ShouldBeEmpty)
			So(first, ShouldResemble, [][]float64{{1, 0}, {0, 1}})
		})

		for _, size := range []float64{-1, 1.5} {
			Convey(fmt.Sprintf("Invalid dimension %g is rejected", size), func() {
				_, err := transport.Evaluate(matrix.NewIdentity(), transport.Values(size))
				So(errors.Is(err, core.ErrShape), ShouldBeTrue)
			})
		}
	})
}
