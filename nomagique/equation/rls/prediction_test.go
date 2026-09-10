package rls_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/equation/rls"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestPredictionNext(t *testing.T) {
	Convey("Given a posterior and two materially different query designs", t, func() {
		node := rls.NewPrediction()
		first, err := transport.Evaluate(node, transport.Values(rls.State{
			Beta: []float64{1, 2}, Root: [][]float64{{1, 0}, {0, 2}},
			NoiseShape: 2, NoiseScale: 4, Design: []float64{1, 3}, Observations: 1,
		}))
		So(err, ShouldBeNil)
		So(first.Prediction, ShouldEqual, 7)
		So(first.Scale, ShouldAlmostEqual, math.Sqrt(76))

		second, err := transport.Evaluate(node, transport.Values(rls.State{
			Beta: []float64{1, 2}, Root: [][]float64{{1, 0}, {0, 2}},
			NoiseShape: 2, NoiseScale: 4, Design: []float64{1, -1}, Observations: 1,
		}))
		So(err, ShouldBeNil)
		So(second.Prediction, ShouldEqual, -1)
		So(second.Scale, ShouldAlmostEqual, math.Sqrt(12))
		So(first.Factor, ShouldResemble, []float64{1, 6})

		Convey("A ragged posterior is rejected instead of partially projected", func() {
			_, err := transport.Evaluate(node, transport.Values(rls.State{
				Beta: []float64{1, 2}, Root: [][]float64{{1}, {0, 2}},
				Design: []float64{1, 3},
			}))
			So(err, ShouldNotBeNil)
		})
	})
}
