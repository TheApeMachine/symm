package correlation_test

import (
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"math"
	"testing"
)

func TestFisherEstimatorNext(t *testing.T) {
	moments := algo.NewWelford()
	node := correlation.NewFisherEstimator(equation.NewCausalResidual(moments))
	for _, x := range []float64{.2, .3, 1, -1, math.NaN(), .5} {
		out := tests.Drain(t, node, tests.Values(x))
		tests.Sound(t, node)
		f := tests.Fields(t, out[0])
		valid := x > -1 && x < 1
		if core.To[bool](f["defined"]) != valid {
			t.Fatal(x)
		}
		if !valid {
			tests.EqualNumber(t, tests.Number(t, tests.Fields(t, moments.Read()), "count"), 2)
		}
		if x == .5 {
			tests.EqualNumber(t, tests.Number(t, f, "baseline"), math.Tanh((math.Atanh(.2)+math.Atanh(.3))/2))
			tests.EqualNumber(t, tests.Number(t, f, "prior_count"), 2)
		}
	}
}
