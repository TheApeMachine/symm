package tests

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
)

// CheckMapReduce pins both arity and independent runs at the shared oracle.
func CheckMapReduce(t *testing.T, node core.Primitive, operation string) {
	t.Helper()
	for _, run := range [][]float64{{1, 2, 3}, {}, {4}, {-2, 5}, {math.NaN()}} {
		payloads := make([]any, len(run))
		expected := 0.0
		for index, value := range run {
			payloads[index] = value
			switch operation {
			case "square-sum":
				expected += value * value
			case "count":
				expected++
			default:
				t.Fatalf("unknown reference %q", operation)
			}
		}
		values := Drain(t, node, Values(payloads...))
		Sound(t, node)
		if len(values) != 1 {
			t.Fatalf("%s yielded %d results", operation, len(values))
		}
		EqualNumber(t, values[0], expected)
	}
}
