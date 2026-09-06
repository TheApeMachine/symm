package algo_test

import (
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
)

func TestGaussJordanNext(t *testing.T) {
	node := algo.NewGaussJordan(store.NewConstant(core.From(1e-15)))
	tests.CheckSolve(t, node)
	output := tests.Drain(t, node, tests.Values(tests.Record(map[string]any{
		"left": [][]float64{{1, 2}, {2, 4}}, "right": [][]float64{{1}, {2}},
	})))
	tests.Sound(t, node)
	if core.To[bool](tests.Fields(t, output[0])["defined"]) {
		t.Fatal("singular matrix declared defined")
	}
	// A later independent solve must not inherit a previous singular status.
	tests.CheckSolve(t, node)
}
