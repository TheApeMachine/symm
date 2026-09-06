package hawkes_test

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation/hawkes"
	"github.com/theapemachine/symm/nomagique/tests"
	"math"
	"math/rand"
	"testing"
)

func TestBranchingNext(t *testing.T) {
	node := hawkes.NewBranching()
	rng := rand.New(rand.NewSource(734))
	for i := 0; i < 30; i++ {
		a, b, c, d := 0.25*rng.Float64(), 0.25*rng.Float64(), 0.25*rng.Float64(), 0.25*rng.Float64()
		beta, muX, muY := 0.5+rng.Float64(), 0.2+rng.Float64(), 0.2+rng.Float64()
		f := tests.Fields(t, tests.Drain(t, node, tests.Values(tests.Record(map[string]any{"mu_x": muX, "mu_y": muY, "alpha_xx": a * beta, "alpha_xy": b * beta, "alpha_yx": c * beta, "alpha_yy": d * beta, "beta": beta})))[0])
		tests.Sound(t, node)
		determinant := (1-a)*(1-d) - b*c
		tests.EqualNumber(t, tests.Number(t, f, "spectral_radius"), (a+d+math.Sqrt((a-d)*(a-d)+4*b*c))/2)
		tests.EqualNumber(t, tests.Number(t, f, "mean_x"), ((1-d)*muX+b*muY)/determinant)
		tests.EqualNumber(t, tests.Number(t, f, "mean_y"), (c*muX+(1-a)*muY)/determinant)
		tests.EqualNumber(t, tests.Number(t, f, "descendants_x"), (1-d+c)/determinant-1)
		tests.EqualNumber(t, tests.Number(t, f, "descendants_y"), (1-a+b)/determinant-1)
		if !core.To[bool](f["defined"]) {
			t.Fatal("stable system undefined")
		}
	}
	f := tests.Fields(t, tests.Drain(t, node, tests.Values(tests.Record(map[string]any{"mu_x": 1.0, "mu_y": 1.0, "alpha_xx": 1.0, "alpha_xy": 0.0, "alpha_yx": 0.0, "alpha_yy": 0.5, "beta": 1.0})))[0])
	tests.Sound(t, node)
	if core.To[bool](f["defined"]) || !math.IsNaN(tests.Number(t, f, "mean_x")) {
		t.Fatal("critical system invented a stationary mean")
	}
}
