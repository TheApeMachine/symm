package tests

import (
	"github.com/theapemachine/symm/nomagique/core"
	"math"
	"math/rand"
	"testing"
)

// CheckOLS compares coefficient, covariance and residual statistics against the
// original independent LU normal-equations fit on deterministic noisy designs.
func CheckOLS(t *testing.T, node core.Primitive) {
	t.Helper()
	random := rand.New(rand.NewSource(471))
	for trial := 0; trial < 35; trial++ {
		parameters := 1 + trial%4
		observations := parameters + 2 + trial%11
		x := make([][]float64, observations)
		flat := make([]float64, 0, observations*parameters)
		y := make([]float64, observations)
		for row := range observations {
			x[row] = make([]float64, parameters)
			for column := range parameters {
				x[row][column] = random.NormFloat64()
				if column == 0 {
					x[row][column] = 1
				}
				y[row] += float64(column+1) * x[row][column]
			}
			y[row] += 0.2 * random.NormFloat64()
			flat = append(flat, x[row]...)
		}
		expected := referenceFitOLS(flat, y, parameters)
		out := Drain(t, node, Values(Record(map[string]any{"x": x, "y": y})))
		Sound(t, node)
		if len(out) != 1 {
			t.Fatalf("trial %d: outputs %d", trial, len(out))
		}
		fields := Fields(t, out[0])
		if core.To[bool](fields["defined"]) != expected.Defined {
			t.Fatalf("trial %d defined mismatch", trial)
		}
		coefficients := core.To[[]float64](fields["coefficients"])
		variance := core.To[[]float64](fields["coefficient_variance"])
		if len(coefficients) != parameters || len(variance) != parameters {
			t.Fatal("coefficient shape")
		}
		for i := range coefficients {
			EqualNumber(t, coefficients[i], expected.Coefficients[i])
			EqualNumber(t, variance[i], expected.CoefficientVariance[i])
		}
		EqualNumber(t, Number(t, fields, "residual_sse"), expected.ResidualSSE)
		EqualNumber(t, Number(t, fields, "residual_variance"), expected.ResidualVariance)
	}
	for _, test := range []struct {
		x [][]float64
		y []float64
	}{
		{[][]float64{{1, 1}, {1, 1}, {1, 1}}, []float64{1, 2, 3}},
		{[][]float64{{1, 0}, {1, 1}}, []float64{1, 2}},
		{[][]float64{}, []float64{}},
	} {
		out := Drain(t, node, Values(Record(map[string]any{"x": test.x, "y": test.y})))
		Sound(t, node)
		fields := Fields(t, out[0])
		if core.To[bool](fields["defined"]) || len(core.To[[]float64](fields["coefficients"])) != 0 || !math.IsNaN(Number(t, fields, "residual_variance")) {
			t.Fatal("undefined fit manufactured evidence")
		}
	}
}
