package probability_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestGeometricMeanComposition(t *testing.T) {
	Convey("Geometric mean follows exp(mean(log x))", t, func() {
		many := make([]float64, 500)

		for index := range many {
			many[index] = 1e-3
		}

		for _, test := range []struct {
			values []float64
			want   float64
		}{
			{[]float64{1, 2, 4}, 2},
			{[]float64{1, 2, 0, 4}, 0},
			{[]float64{1, -2}, math.NaN()},
			{[]float64{.1, .9}, .3},
			{many, 1e-3},
		} {
			node := probability.NewGeometricMean()
			out := tests.CollectSeq[float64](node.Next(transport.NewValues(test.values...).Next(nil)))
			So(node.Error(), ShouldBeNil)
			So(len(out), ShouldBeGreaterThan, 0)

			if math.IsNaN(test.want) {
				So(math.IsNaN(out[len(out)-1]), ShouldBeTrue)
			}

			if !math.IsNaN(test.want) {
				So(out[len(out)-1], ShouldAlmostEqual, test.want, 1e-12)
			}
		}

		empty := tests.CollectSeq[float64](probability.NewGeometricMean().Next(transport.NewValues[float64]().Next(nil)))
		So(len(empty), ShouldEqual, 0)
	})
}

func TestAmbiguityComposition(t *testing.T) {
	Convey("Ambiguity is normalized Shannon entropy of the simplex", t, func() {
		for _, test := range []struct {
			values []float64
			want   float64
		}{
			{[]float64{1, 1, 1, 1}, 1},
			{[]float64{5, 0, 0}, 0},
			{[]float64{7}, 0},
			{[]float64{0, 0}, 0},
			{[]float64{1, 1, 2}, 1.5 * math.Ln2 / math.Log(3)},
			{[]float64{.25, .25, .5}, 1.5 * math.Ln2 / math.Log(3)},
		} {
			outEval := transport.NewEvaluate(probability.NewAmbiguity())
			var out float64

			for res := range outEval.Next(transport.NewValues(test.values...).Next(nil)) {
				out = *(*float64)(res)
			}

			err := outEval.Error()
			So(err, ShouldBeNil)
			So(out, ShouldAlmostEqual, test.want, 1e-12)
		}
	})
}

func TestSelectionComposition(t *testing.T) {
	Convey("Argmax keeps the first maximum and emits nothing for an empty run", t, func() {
		node := probability.NewArgmax()

		for _, test := range []struct {
			values []float64
			index  int
			value  float64
		}{
			{[]float64{1, 9, 3}, 1, 9},
			{[]float64{4, 4}, 0, 4},
		} {
			outEval := transport.NewEvaluate(node)
			var out probability.ArgmaxResult

			for res := range outEval.Next(transport.NewValues(test.values...).Next(nil)) {
				out = *(*probability.ArgmaxResult)(res)
			}

			err := outEval.Error()
			So(err, ShouldBeNil)
			So(out.Index, ShouldEqual, test.index)
			So(out.Value, ShouldEqual, test.value)
		}

		empty := tests.CollectSeq[probability.ArgmaxResult](node.Next(transport.NewValues[float64]().Next(nil)))
		So(node.Error(), ShouldBeNil)
		So(len(empty), ShouldEqual, 0)
	})
}

func TestNewGeomean(t *testing.T) {
	Convey("Geomean is the GeometricMean recurrence as a reduction Primitive", t, func() {
		out := tests.CollectSeq[float64](probability.NewGeomean().Next(transport.NewValues(1.0, 2.0, 4.0).Next(nil)))

		So(len(out), ShouldEqual, 3)
		So(out[0], ShouldAlmostEqual, 1, 1e-12)
		So(out[1], ShouldAlmostEqual, math.Sqrt(2), 1e-12)
		So(out[2], ShouldAlmostEqual, 2, 1e-12)
	})
}

func TestNewShannonAmbiguity(t *testing.T) {
	Convey("ShannonAmbiguity yields the running normalized entropy after every arrival", t, func() {
		out := tests.CollectSeq[float64](probability.NewShannonAmbiguity().Next(transport.NewValues(1.0, 1.0, 1.0, 1.0).Next(nil)))

		So(len(out), ShouldEqual, 4)
		So(out[0], ShouldEqual, 0)
		So(out[1], ShouldAlmostEqual, 1, 1e-12)
		So(out[2], ShouldAlmostEqual, 1, 1e-12)
		So(out[3], ShouldAlmostEqual, 1, 1e-12)
	})

	Convey("A skewed run converges to its normalized entropy", t, func() {
		out := tests.CollectSeq[float64](probability.NewShannonAmbiguity().Next(transport.NewValues(0.25, 0.25, 0.5).Next(nil)))

		So(len(out), ShouldEqual, 3)
		So(out[2], ShouldAlmostEqual, 1.5*math.Ln2/math.Log(3), 1e-12)
	})

	Convey("A zero-total run stays at zero", t, func() {
		out := tests.CollectSeq[float64](probability.NewShannonAmbiguity().Next(transport.NewValues(0.0, 0.0).Next(nil)))

		So(out[0], ShouldEqual, 0)
		So(out[1], ShouldEqual, 0)
	})
}
