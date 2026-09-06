package tests

import (
	"github.com/theapemachine/symm/nomagique/core"
	"math"
	"testing"
)

// CheckDistribution replaces receiver-specific checks with the same numerical
// properties over Primitive records. The old zero-allocation contract does not
// apply to the explicitly buffered graph and is not asserted here.
func CheckDistribution(t *testing.T, node core.Primitive) {
	t.Helper()
	var first map[string]core.Primitive
	for _, run := range [][]float64{{1, 1, 1, 1}, {0, 5, 1}, {1000, 1001}, {8}, {0, 4}} {
		maximum := math.Inf(-1)
		payloads := make([]any, len(run))
		for i, x := range run {
			payloads[i] = x
			maximum = math.Max(maximum, x)
		}
		out := Drain(t, node, Values(payloads...))
		Sound(t, node)
		if len(out) != 1 {
			t.Fatal("distribution must publish one record")
		}
		f := Fields(t, out[0])
		expected := make([]float64, len(run))
		total := 0.0
		winner := 0
		for i, x := range run {
			expected[i] = math.Exp(x - maximum)
			total += expected[i]
			if x > run[winner] {
				winner = i
			}
		}
		actual := core.To[[]float64](f["probabilities"])
		entropy := 0.0
		if len(actual) != len(expected) {
			t.Fatal("simplex shape")
		}
		for i := range expected {
			expected[i] /= total
			EqualNumber(t, actual[i], expected[i])
			if expected[i] > 0 {
				entropy -= expected[i] * math.Log(expected[i])
			}
		}
		ambiguity := 0.0
		if len(run) > 1 {
			ambiguity = entropy / math.Log(float64(len(run)))
		}
		EqualNumber(t, Number(t, f, "winner"), float64(winner))
		EqualNumber(t, Number(t, f, "confidence"), expected[winner])
		EqualNumber(t, Number(t, f, "ambiguity"), ambiguity)
		EqualNumber(t, Number(t, f, "sharpness"), 1-ambiguity)
		if first == nil {
			first = f
		}
	}
	EqualNumber(t, Number(t, first, "ambiguity"), 1)
	EqualNumber(t, Number(t, first, "confidence"), .25)
}
