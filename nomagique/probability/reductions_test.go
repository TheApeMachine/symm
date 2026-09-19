package probability_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/probability"
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
			var out float64
			for _, v := range test.values {
				out = node(v)
			}

			if math.IsNaN(test.want) {
				So(math.IsNaN(out), ShouldBeTrue)
			} else {
				So(out, ShouldAlmostEqual, test.want, 1e-12)
			}
		}
	})
}

func TestAmbiguityComposition(t *testing.T) {
	Convey("Ambiguity is normalized Shannon entropy of the simplex", t, func() {
		ambiguity := probability.NewAmbiguity()

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
			out := ambiguity(test.values)
			So(out, ShouldAlmostEqual, test.want, 1e-12)
		}
	})
}

func TestSelectionComposition(t *testing.T) {
	Convey("Argmax keeps the first maximum and emits nothing for an empty run", t, func() {
		argmax := probability.NewArgmax()

		for _, test := range []struct {
			values []float64
			index  int
			value  float64
		}{
			{[]float64{1, 9, 3}, 1, 9},
			{[]float64{4, 4}, 0, 4},
		} {
			out := argmax(test.values)
			So(out.Index, ShouldEqual, test.index)
			So(out.Value, ShouldEqual, test.value)
		}

		empty := argmax(nil)
		So(empty.Index, ShouldEqual, 0)
		So(empty.Value, ShouldEqual, 0)
	})
}

func TestNewGeomean(t *testing.T) {
	Convey("Geomean is the GeometricMean recurrence as a Value closure", t, func() {
		geomean := probability.NewGeomean()
		inputs := []float64{core.Unit, 2.0, 4.0}
		out := make([]float64, len(inputs))
		for i, v := range inputs {
			out[i] = geomean(v)
		}

		So(out[0], ShouldAlmostEqual, 1, 1e-12)
		So(out[1], ShouldAlmostEqual, math.Sqrt(2), 1e-12)
		So(out[2], ShouldAlmostEqual, 2, 1e-12)
	})
}

func TestNewShannonAmbiguity(t *testing.T) {
	Convey("ShannonAmbiguity yields the running normalized entropy after every arrival", t, func() {
		amb := probability.NewShannonAmbiguity()
		inputs := []float64{core.Unit, core.Unit, core.Unit, core.Unit}
		out := make([]float64, len(inputs))
		for i, v := range inputs {
			out[i] = amb(v)
		}

		So(out[0], ShouldEqual, 0)
		So(out[1], ShouldAlmostEqual, 1, 1e-12)
		So(out[2], ShouldAlmostEqual, 1, 1e-12)
		So(out[3], ShouldAlmostEqual, 1, 1e-12)
	})

	Convey("A skewed run converges to its normalized entropy", t, func() {
		amb := probability.NewShannonAmbiguity()
		inputs := []float64{0.25, 0.25, 0.5}
		var out float64
		for _, v := range inputs {
			out = amb(v)
		}

		So(out, ShouldAlmostEqual, 1.5*math.Ln2/math.Log(3), 1e-12)
	})

	Convey("A zero-total run stays at zero", t, func() {
		amb := probability.NewShannonAmbiguity()
		inputs := []float64{0.0, 0.0}
		out := make([]float64, len(inputs))
		for i, v := range inputs {
			out[i] = amb(v)
		}

		So(out[0], ShouldEqual, 0)
		So(out[1], ShouldEqual, 0)
	})
}
