package matrix_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestScaleNext(t *testing.T) {
	Convey("Scale multiplies every coefficient without mutating the source", t, func() {
		node := matrix.NewScale()
		values := [][]float64{{1, -2}, {}, {3}}
		first, err := transport.Evaluate(node, transport.Values(matrix.ScaleInput{Values: values, Factor: -2}))
		So(err, ShouldBeNil)
		So(first, ShouldResemble, [][]float64{{-2, 4}, {}, {-6}})

		second, err := transport.Evaluate(node, transport.Values(matrix.ScaleInput{Values: values, Factor: 0}))
		So(err, ShouldBeNil)
		So(second, ShouldResemble, [][]float64{{0, 0}, {}, {0}})
		So(first, ShouldResemble, [][]float64{{-2, 4}, {}, {-6}})
		So(values, ShouldResemble, [][]float64{{1, -2}, {}, {3}})
	})
}
