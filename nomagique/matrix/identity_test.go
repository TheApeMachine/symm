package matrix_test

import (
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
)

func TestIdentityNext(t *testing.T) {
	node := matrix.NewIdentity()
	for _, size := range []int{1, 3, 2} {
		output := tests.Drain(t, node, tests.Values(float64(size)))
		tests.Sound(t, node)
		got := output[0].([][]float64)
		if len(got) != size {
			t.Fatal(got)
		}
		for row := range size {
			for column := range size {
				expected := 0.0
				if row == column {
					expected = 1
				}
				tests.EqualNumber(t, got[row][column], expected)
			}
		}
	}
}
