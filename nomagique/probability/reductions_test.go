package probability_test

import (
	"errors"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
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
			{[]float64{1, 2, 4}, 2}, {[]float64{1, 2, 0, 4}, 0},
			{[]float64{1, -2}, math.NaN()}, {[]float64{.1, .9}, .3}, {many, 1e-3},
		} {
			node := equation.NewGeometricMean[float64]()
			out := tests.CollectSeq(node.Next(transport.Values(test.values...)))
			So(node.Error(), ShouldBeNil)
			So(len(out), ShouldBeGreaterThan, 0)

			if math.IsNaN(test.want) {
				So(math.IsNaN(out[len(out)-1]), ShouldBeTrue)
			}

			if !math.IsNaN(test.want) {
				So(out[len(out)-1], ShouldAlmostEqual, test.want)
			}
		}

		empty := tests.CollectSeq(equation.NewGeometricMean[float64]().Next(transport.Values[float64]()))
		So(len(empty), ShouldEqual, 0)
	})
}

func TestAmbiguityComposition(t *testing.T) {
	Convey("Ambiguity is normalized Shannon entropy of the simplex", t, func() {
		for _, test := range []struct {
			values []float64
			want   float64
		}{
			{[]float64{1, 1, 1, 1}, 1}, {[]float64{5, 0, 0}, 0}, {[]float64{7}, 0},
			{[]float64{0, 0}, math.NaN()},
			{[]float64{1, 1, 2}, 1.5 * math.Ln2 / math.Log(3)},
			{[]float64{.25, .25, .5}, 1.5 * math.Ln2 / math.Log(3)},
		} {
			out, err := transport.Evaluate(probability.NewAmbiguity(), transport.Values(test.values...))

			if math.IsNaN(test.want) {
				So(err, ShouldBeNil)
				So(math.IsNaN(out), ShouldBeTrue)
			}

			if !math.IsNaN(test.want) {
				So(err, ShouldBeNil)
				So(out, ShouldAlmostEqual, test.want)
			}
		}
	})
}

func TestShareComposition(t *testing.T) {
	Convey("Evidence share selects one normalized member", t, func() {
		for index, want := range []float64{.25, .75} {
			out, err := transport.Evaluate(equation.NewEvidenceShare[float64](index), transport.Values(1.0, 3.0))
			So(err, ShouldBeNil)
			So(out, ShouldEqual, want)
		}

		node := equation.NewEvidenceShare[float64](5)
		tests.CollectSeq(node.Next(transport.Values(1.0, 3.0)))
		So(errors.Is(node.Error(), core.ErrShape), ShouldBeTrue)

		out, err := transport.Evaluate(equation.NewEvidenceShare[float64](0), transport.Values(0.0, 0.0))
		So(err, ShouldBeNil)
		So(math.IsNaN(out), ShouldBeTrue)
	})
}

func TestSelectionComposition(t *testing.T) {
	Convey("Argmax keeps the first maximum and emits nothing for an empty run", t, func() {
		node := equation.NewArgmax[float64]()

		for _, test := range []struct {
			values       []float64
			index, value float64
		}{{[]float64{1, 9, 3}, 1, 9}, {[]float64{4, 4}, 0, 4}} {
			out, err := transport.Evaluate(node, transport.Values(test.values...))
			So(err, ShouldBeNil)
			So(out.Index, ShouldEqual, test.index)
			So(out.Value, ShouldEqual, test.value)
		}

		empty := tests.CollectSeq(node.Next(transport.Values[float64]()))
		So(node.Error(), ShouldBeNil)
		So(len(empty), ShouldEqual, 0)

		selected, err := transport.Evaluate(collection.NewAt[float64](1), transport.Values([]float64{1, 9, 3}))
		So(err, ShouldBeNil)
		So(selected, ShouldEqual, 9)
	})
}
