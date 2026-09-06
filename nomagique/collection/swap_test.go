package collection_test

import (
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
)

func TestSwapNext(t *testing.T) {
	node := collection.NewSwap[float64](tests.Values(0.0), tests.Values(2.0))
	for range 3 {
		original := []float64{2, 3, 4}
		output := tests.Drain(t, node, tests.Values(original))
		tests.Sound(t, node)
		for index, value := range []float64{4, 3, 2} {
			tests.EqualNumber(t, output[0].([]float64)[index], value)
		}
		tests.EqualNumber(t, original[0], 2)
	}
}
