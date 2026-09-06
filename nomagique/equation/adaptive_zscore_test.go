package equation_test

import (
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"math"
	"testing"
)

func TestAdaptiveZScoreNext(t *testing.T) {
	node := equation.NewAdaptiveZScore(algo.NewWelford())
	results := tests.Drain(t, node, tests.Values(0.02, 0.01))
	tests.Sound(t, node)
	first, second := tests.Fields(t, results[0]), tests.Fields(t, results[1])
	tests.EqualNumber(t, tests.Number(t, first, "baseline"), 0.02)
	tests.EqualNumber(t, tests.Number(t, first, "zscore"), 0)
	tests.EqualNumber(t, tests.Number(t, second, "baseline"), 0.02)
	tests.EqualNumber(t, tests.Number(t, second, "ratio"), 0.5)
	tests.EqualNumber(t, tests.Number(t, second, "divergence"), math.Log(0.5))
	tests.EqualNumber(t, tests.Number(t, second, "zscore"), -1)
}
