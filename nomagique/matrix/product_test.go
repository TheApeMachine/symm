package matrix_test

import (
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
)

func TestProductNext(t *testing.T) {
	node := matrix.NewProduct(store.NewGet("left"), store.NewGet("right"))
	for range 3 {
		output := tests.Drain(t, node, tests.Values(tests.Record(map[string]any{
			"left": [][]float64{{1, 2, 3}, {4, 5, 6}}, "right": [][]float64{{1, 0}, {0, 1}, {1, 1}},
		})))
		tests.Sound(t, node)
		got := output[0].([][]float64)
		for row, wanted := range [][]float64{{4, 5}, {10, 11}} {
			for column, value := range wanted {
				tests.EqualNumber(t, got[row][column], value)
			}
		}
	}
}
