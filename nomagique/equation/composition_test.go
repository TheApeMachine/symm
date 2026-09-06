package equation_test

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
	"testing"
)

func values(xs ...float64) core.Primitive {
	ps := []core.Primitive{}
	for _, x := range xs {
		ps = append(ps, core.From(x))
	}
	return transport.NewIO(ps...)
}
func record(m map[string]float64) core.Primitive {
	r := map[string]core.Primitive{}
	for k, v := range m {
		r[k] = core.From(v)
	}
	return transport.NewIO(core.From(r))
}
func TestReductions(t *testing.T) {
	cases := []struct {
		node   core.Primitive
		wanted float64
	}{
		{equation.NewCount(), 3}, {equation.NewMean(), 2}, {equation.NewEnergy(), 14}, {equation.NewKish(), 36.0 / 14},
	}
	for _, c := range cases {
		for range 3 {
			tests.EqualNumber(t, tests.Drain(t, c.node, values(1, 2, 3))[0], c.wanted)
			if c.node.Error() != nil {
				t.Fatal(c.node.Error())
			}
		}
	}
}
func TestMedian(t *testing.T) {
	for _, c := range []struct {
		input  []float64
		wanted float64
	}{{[]float64{9, 1, 5}, 5}, {[]float64{9, 1, 5, 3}, 4}, {[]float64{1, math.NaN(), 3}, math.NaN()}, {[]float64{1, 2, math.Inf(1)}, 2}} {
		node := equation.NewMedian()
		out := tests.Drain(t, node, values(c.input...))
		if len(out) != 1 {
			t.Fatal(out)
		}
		tests.EqualNumber(t, out[0], c.wanted)
		if node.Error() != nil {
			t.Fatal(node.Error())
		}
	}
}
func TestExpressionBindings(t *testing.T) {
	node := equation.NewDifference[float64](store.NewGet("a"), store.NewGet("b"))
	for range 3 {
		tests.EqualNumber(t, tests.Drain(t, node, record(map[string]float64{"a": 10, "b": 3}))[0], 7)
	}
}
func TestSigmoid(t *testing.T) {
	node := equation.NewSigmoid()
	out := tests.Drain(t, node, values(0, math.Inf(1), math.Inf(-1)))
	if len(out) != 3 {
		t.Fatal(out)
	}
	tests.EqualNumber(t, out[0], 0.5)
	tests.EqualNumber(t, out[1], 1)
	tests.EqualNumber(t, out[2], 0)
}
func TestFisherDomain(t *testing.T) {
	for _, c := range []struct{ r, n, p float64 }{{0, 103, 1}, {1, 103, 0}, {-1, 103, 0}, {2, 103, math.NaN()}, {0.5, 2, math.NaN()}} {
		node := equation.NewFisher()
		out := tests.Drain(t, node, record(map[string]float64{"correlation": c.r, "support": c.n}))
		if len(out) != 1 {
			t.Fatal(out)
		}
		tests.EqualNumber(t, out[0], c.p)
	}
}
func TestNormalize(t *testing.T) {
	node := equation.NewNormalize()
	out := tests.Drain(t, node, values(1, 1, 2))
	for i, want := range []float64{0.25, 0.25, 0.5} {
		tests.EqualNumber(t, out[i], want)
	}
}
