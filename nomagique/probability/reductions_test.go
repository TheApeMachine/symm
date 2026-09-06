package probability_test

import (
	"errors"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
	"testing"
)

func TestGeometricMeanComposition(t *testing.T) {
	node := transport.NewPipe(transport.NewSpread[float64](), equation.NewGeometricMean())
	many := make([]float64, 500)
	for i := range many {
		many[i] = 1e-3
	}
	for _, c := range []struct {
		values []float64
		want   float64
	}{
		{[]float64{1, 2, 4}, 2}, {[]float64{1, 2, 0, 4}, 0}, {nil, math.NaN()},
		{[]float64{1, -2}, math.NaN()}, {[]float64{.1, .9}, .3}, {many, 1e-3},
	} {
		out := tests.Drain(t, node, tests.Values(c.values))
		tests.Sound(t, node)
		tests.EqualNumber(t, out[0], c.want)
	}
}
func TestAmbiguityComposition(t *testing.T) {
	node := transport.NewPipe(transport.NewSpread[float64](), probability.NewAmbiguity())
	for _, c := range []struct {
		values []float64
		want   float64
	}{
		{[]float64{1, 1, 1, 1}, 1}, {[]float64{5, 0, 0}, 0}, {[]float64{7}, 0},
		{[]float64{0, 0}, math.NaN()},
		{[]float64{1, 1, 2}, 1.5 * math.Ln2 / math.Log(3)},
		{[]float64{.25, .25, .5}, 1.5 * math.Ln2 / math.Log(3)},
	} {
		out := tests.Drain(t, node, tests.Values(c.values))
		tests.Sound(t, node)
		tests.EqualNumber(t, out[0], c.want)
	}
}
func TestShareComposition(t *testing.T) {
	for i, want := range []float64{.25, .75} {
		node := equation.NewEvidenceShare(store.NewConstant(core.From(float64(i))))
		out := tests.Drain(t, node, tests.Values(1.0, 3.0))
		tests.Sound(t, node)
		tests.EqualNumber(t, out[0], want)
	}
	node := equation.NewEvidenceShare(store.NewConstant(core.From(5.0)))
	tests.Drain(t, node, tests.Values(1.0, 3.0))
	if !errors.Is(node.Error(), core.ErrShape) {
		t.Fatal("missing share was silently zero")
	}
	node = equation.NewEvidenceShare(store.NewConstant(core.From(0.0)))
	out := tests.Drain(t, node, tests.Values(0.0, 0.0))
	tests.Sound(t, node)
	tests.EqualNumber(t, out[0], math.NaN())
}
func TestSelectionComposition(t *testing.T) {
	node := equation.NewArgmax()
	for _, c := range []struct {
		v            []float64
		index, value float64
	}{{[]float64{1, 9, 3}, 1, 9}, {[]float64{4, 4}, 0, 4}} {
		source := transport.NewApply(transport.NewSpread[float64](), tests.Values(c.v))
		out := tests.Drain(t, node, source)
		tests.Sound(t, node)
		f := tests.Fields(t, out[0])
		tests.EqualNumber(t, tests.Number(t, f, "index"), c.index)
		tests.EqualNumber(t, tests.Number(t, f, "value"), c.value)
	}
	out := tests.Drain(t, node, tests.Values[float64]())
	tests.Sound(t, node)
	if len(out) != 0 {
		t.Fatal("empty selection fabricated a record", out)
	}
	// A selected Primitive can feed the ordinary indexed selection operation.
	selector := collection.NewAt[float64](transport.NewIO(core.From(1.0)))
	tests.EqualNumber(t, tests.Drain(t, selector, tests.Values([]float64{1, 9, 3}))[0], 9)
}
