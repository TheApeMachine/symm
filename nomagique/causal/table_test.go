package causal_test

import (
	"github.com/theapemachine/symm/nomagique/causal"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"math"
	"testing"
)

func TestBackdoorNext(t *testing.T) {
	node := causal.NewBackdoor(causal.NewLinearFit(store.NewConstant(core.From(1e-15))), causal.NewLinearPrediction())
	tests.CheckInterventions(t, node, false)
}
func TestCounterfactualNext(t *testing.T) {
	node := causal.NewCounterfactual(causal.NewLinearFit(store.NewConstant(core.From(1e-15))), causal.NewLinearPrediction(), causal.NewLinearPrediction())
	tests.CheckInterventions(t, node, true)
}
func TestCounterfactualRankDeficiency(t *testing.T) {
	node := causal.NewCounterfactual(causal.NewLinearFit(store.NewConstant(core.From(1e-15))), causal.NewLinearPrediction(), causal.NewLinearPrediction())
	out := tests.Drain(t, node, tests.Values(tests.Record(map[string]any{
		"rows": [][]float64{{1, 0, 2}, {1, 0, 3}, {1, 1, 4}, {1, 1, 5}}, "features": []float64{0, 1}, "target": 2.0, "treatment": 1.0, "level": 1.0, "actual": []float64{1, 0, 2}})))
	tests.Sound(t, node)
	f := tests.Fields(t, out[0])
	if core.To[bool](f["defined"]) || !math.IsNaN(tests.Number(t, f, "counterfactual")) {
		t.Fatal("singular fit invented a counterfactual")
	}
}
