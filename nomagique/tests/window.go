package tests

import (
	"github.com/theapemachine/symm/nomagique/core"
	"math"
	"math/rand"
	"testing"
)

// CheckWindow compares each transition to the supplied independent reference,
// not merely the final stationary/drift properties.
func CheckWindow(t *testing.T, node core.Primitive) {
	t.Helper()
	random := rand.New(rand.NewSource(11))
	reference := &referenceWindow{Type: ADWIN}
	for index := 0; index < 1200; index++ {
		level := 1.0
		if index >= 500 && index < 950 {
			level = 5.0
		}
		value := level + random.NormFloat64()*0.1
		expected := reference.Step(value)
		output := Drain(t, node, Values(value))
		Sound(t, node)
		if len(output) != 1 {
			t.Fatalf("at %d: %d outputs", index, len(output))
		}
		fields := Fields(t, output[0])
		EqualNumber(t, Number(t, fields, "capacity"), float64(expected))
		EqualNumber(t, Number(t, fields, "shed_ratio"), reference.ShedRatio())
		all := Fields(t, core.To[any](fields["all"]))
		EqualNumber(t, Number(t, all, "mean"), reference.welford.Mean())
		if math.Abs(Number(t, all, "count")-reference.welford.Count()) > 1e-9 {
			t.Fatalf("support mismatch at %d", index)
		}
	}
}
func CheckCausalResidual(t *testing.T, node core.Primitive, adaptive bool) {
	t.Helper()
	random := rand.New(rand.NewSource(7))
	var moments referenceMoments
	var window referenceWindow
	for index := 0; index < 1000; index++ {
		level := []float64{0.02, 0.14, 0.05, 0.22, 0.08, 0.17, 0.04, 0.11}[index/125]
		value := level + random.NormFloat64()*0.02
		priorCount, priorMean, priorDispersion := moments.Count(), moments.Mean(), moments.Dispersion()
		moments.Update(value)
		if adaptive {
			window.Step(value)
			moments.Shed(window.ShedRatio())
		}
		baseline, residual, score, maturity := priorMean, value-priorMean, 0.0, 1.0-1.0/(priorCount+1.0)
		if priorCount == 0 {
			baseline = value
			residual = 0
		}
		scale := priorDispersion
		if scale <= 0 {
			scale = math.Abs(residual)
		}
		if scale > 0 {
			score = residual / scale
		}
		result := Drain(t, node, Values(value))
		Sound(t, node)
		if len(result) != 1 {
			t.Fatal(result)
		}
		fields := Fields(t, result[0])
		EqualNumber(t, Number(t, fields, "baseline"), baseline)
		EqualNumber(t, Number(t, fields, "residual"), residual)
		EqualNumber(t, Number(t, fields, "zscore"), score)
		EqualNumber(t, Number(t, fields, "maturity"), maturity)
		EqualNumber(t, Number(t, fields, "count"), moments.Count())
		EqualNumber(t, Number(t, fields, "mean"), moments.Mean())
	}
}
