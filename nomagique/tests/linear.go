package tests

import (
	"github.com/theapemachine/symm/nomagique/core"
	"math"
	"math/rand"
	"testing"
)

// CheckSolve checks known solutions and inverses by multiplying independent
// test matrices. No production solver is used to manufacture expected answers.
func CheckSolve(t *testing.T, node core.Primitive) {
	t.Helper()
	random := rand.New(rand.NewSource(192))
	for trial := 0; trial < 30; trial++ {
		size := 1 + trial%5
		left := make([][]float64, size)
		right := make([][]float64, size)
		expected := make([]float64, size)
		for row := range size {
			expected[row] = random.NormFloat64()
			left[row] = make([]float64, size)
			right[row] = make([]float64, 1+size)
			for column := range size {
				left[row][column] = random.NormFloat64()
			}
			left[row][row] += float64(size) + 2
		}
		for row := range size {
			for column := range size {
				right[row][0] += left[row][column] * expected[column]
			}
		}
		if trial%2 == 0 && size > 1 {
			left[0], left[size-1] = left[size-1], left[0]
			right[0], right[size-1] = right[size-1], right[0]
		}
		// Attach identity after permuting the equations: these right-hand
		// sides request the inverse of the matrix actually sent to the solver.
		for row := range size {
			right[row][row+1] = 1
		}
		out := Drain(t, node, Values(Record(map[string]any{"left": left, "right": right})))
		Sound(t, node)
		if len(out) != 1 {
			t.Fatalf("got %d solutions", len(out))
		}
		fields := Fields(t, out[0])
		if !core.To[bool](fields["defined"]) {
			t.Fatalf("trial %d undefined", trial)
		}
		solution := core.To[[][]float64](fields["solution"])
		for row := range size {
			EqualNumber(t, solution[row][0], expected[row])
		}
		for row := range size {
			for column := range size {
				product := 0.0
				for inner := range size {
					product += left[row][inner] * solution[inner][column+1]
				}
				target := 0.0
				if row == column {
					target = 1
				}
				if math.Abs(product-target) > 1e-11 {
					t.Fatalf("inverse residual %g", product-target)
				}
			}
		}
	}
}
