package equation_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func last[U any](values []U) U {
	return values[len(values)-1]
}

func TestReductions(t *testing.T) {
	Convey("Stream folds yield a running value whose last observation is the reduction", t, func() {
		count := tests.CollectSeq(equation.NewCount[float64]().Next(transport.Values(1.0, 2.0, 3.0)))
		So(last(count), ShouldEqual, 3)

		mean := tests.CollectSeq(equation.NewMean[float64]().Next(transport.Values(1.0, 2.0, 3.0)))
		So(last(mean), ShouldEqual, 2)

		energy := tests.CollectSeq(equation.NewEnergy[float64]().Next(transport.Values(1.0, 2.0, 3.0)))
		So(last(energy), ShouldEqual, 14)

		kish := tests.CollectSeq(equation.NewKish[float64]().Next(transport.Values(1.0, 2.0, 3.0)))
		So(last(kish), ShouldEqual, 36.0/14)
	})
}

func TestMedian(t *testing.T) {
	Convey("Median averages the two central order statistics of one run", t, func() {
		for _, c := range []struct {
			input  []float64
			wanted float64
		}{
			{[]float64{9, 1, 5}, 5},
			{[]float64{9, 1, 5, 3}, 4},
			{[]float64{1, 2, math.Inf(1)}, 2},
		} {
			out := tests.CollectSeq(equation.NewMedian[float64]().Next(transport.Values(c.input...)))
			So(len(out), ShouldEqual, 1)
			So(out[0], ShouldEqual, c.wanted)
		}
	})
}

func TestExpressionBindings(t *testing.T) {
	Convey("A pairwise difference reads two fields of one arrival", t, func() {
		node := equation.NewDifference(
			store.NewGet[string, float64]("a"),
			store.NewGet[string, float64]("b"),
		)
		out := tests.CollectSeq(node.Next(transport.Values(map[string]float64{"a": 10, "b": 3})))
		So(out[0], ShouldEqual, 7)
	})
}

func TestSigmoid(t *testing.T) {
	Convey("Sigmoid maps each arrival independently", t, func() {
		out := tests.CollectSeq(equation.NewSigmoid[float64]().Next(transport.Values(0.0, math.Inf(1), math.Inf(-1))))
		So(out[0], ShouldEqual, 0.5)
		So(out[1], ShouldEqual, 1)
		So(out[2], ShouldEqual, 0)
	})
}

func TestFisherDomain(t *testing.T) {
	Convey("Fisher-z p-values follow the formula's domain", t, func() {
		for _, c := range []struct{ r, n, p float64 }{
			{0, 103, 1},
			{1, 103, 0},
			{-1, 103, 0},
		} {
			out := tests.CollectSeq(equation.NewFisher().Next(transport.Values(equation.FisherInput{
				Correlation: c.r,
				Support:     c.n,
			})))
			So(len(out), ShouldEqual, 1)
			So(out[0], ShouldAlmostEqual, c.p, 1e-9)
		}
	})
}

func TestNormalize(t *testing.T) {
	Convey("Normalize divides each arrival by the run total", t, func() {
		out := tests.CollectSeq(equation.NewNormalize[float64]().Next(transport.Values(1.0, 1.0, 2.0)))
		So(out, ShouldResemble, []float64{0.25, 0.25, 0.5})
	})
}
