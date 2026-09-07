package rls_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation/rls"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestPredictionNext(t *testing.T) {
	Convey("Given a posterior and two materially different query designs", t, func() {
		posterior := map[string]any{
			"beta": []float64{1, 2}, "root": [][]float64{{1, 0}, {0, 2}},
			"noise_shape": 2.0, "noise_scale": 4.0, "design": []float64{1, 3},
		}
		node := rls.NewPrediction(store.NewConstant(core.From(1.0)))
		first, err := transport.Evaluate[map[string]core.Primitive](node, core.Record(posterior))
		So(err, ShouldBeNil)
		So(tests.Number(t, first, "prediction"), ShouldEqual, 7)
		So(tests.Number(t, first, "scale"), ShouldAlmostEqual, math.Sqrt(76))
		posterior["design"] = []float64{1, -1}
		second, err := transport.Evaluate[map[string]core.Primitive](node, core.Record(posterior))
		So(err, ShouldBeNil)
		So(tests.Number(t, second, "prediction"), ShouldEqual, -1)
		So(tests.Number(t, second, "scale"), ShouldAlmostEqual, math.Sqrt(12))
		So(core.To[[]float64](first["factor"]), ShouldResemble, []float64{1, 6})

		Convey("A ragged posterior is rejected instead of partially projected", func() {
			posterior["root"] = [][]float64{{1}, {0, 2}}
			_, err := transport.Evaluate[map[string]core.Primitive](node, core.Record(posterior))
			So(err, ShouldNotBeNil)
		})
	})
}
