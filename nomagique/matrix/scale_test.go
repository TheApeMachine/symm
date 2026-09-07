package matrix_test

import (
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestScaleNext(t *testing.T) {
	Convey("Given changing matrix and scalar expressions", t, func() {
		node := matrix.NewScale(store.NewGet("values"), store.NewGet("scale"))
		values := [][]float64{{1, -2}, {}, {3}}
		first, err := transport.Evaluate[[][]float64](node, tests.Record(map[string]any{"values": values, "scale": -2.0}))
		So(err, ShouldBeNil)
		So(first, ShouldResemble, [][]float64{{-2, 4}, {}, {-6}})

		Convey("A zero multiplier produces zeros without overwriting prior output", func() {
			second, err := transport.Evaluate[[][]float64](node, tests.Record(map[string]any{"values": values, "scale": 0.0}))
			So(err, ShouldBeNil)
			So(second, ShouldResemble, [][]float64{{0, 0}, {}, {0}})
			So(first, ShouldResemble, [][]float64{{-2, 4}, {}, {-6}})
			So(values, ShouldResemble, [][]float64{{1, -2}, {}, {3}})
			So(core.To[[][]float64](node), ShouldResemble, second)
		})
	})
}
