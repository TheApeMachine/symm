package tests

import (
	"github.com/theapemachine/symm/nomagique/core"
	"testing"
)

// CheckInterventions derives outcomes from an explicit affine structural model.
// It tests repeated levels and retention of factual noise without using a
// production estimator to manufacture reference predictions.
func CheckInterventions(t *testing.T, node core.Primitive, counterfactual bool) {
	t.Helper()
	rows := [][]float64{{0, 0, 1}, {1, 0, 2}, {0, 1, 4}, {1, 1, 5}, {2, 0, 3}, {2, 1, 6}}
	for _, level := range []float64{1, 0, 2, -1} {
		for _, noise := range []float64{0, 0.25, -0.5} {
			input := Record(map[string]any{"rows": rows, "features": []float64{0, 1}, "target": 2.0, "treatment": 1.0, "level": level, "actual": []float64{0, 0, 1 + noise}})
			out := Drain(t, node, Values(input))
			Sound(t, node)
			f := Fields(t, out[0])
			if !core.To[bool](f["defined"]) {
				t.Fatal("defined affine design rejected")
			}
			if counterfactual {
				EqualNumber(t, Number(t, f, "counterfactual"), 1+3*level+noise)
				EqualNumber(t, Number(t, f, "noise"), noise)
			} else {
				EqualNumber(t, Number(t, f, "expectation"), 2+3*level)
			}
		}
	}
}
