package tests

import (
	"github.com/theapemachine/symm/nomagique/core"
	"math"
	"testing"
)

// CheckCalibrator compares rank before insertion with the preserved ring oracle.
// Reset is supplied by a new graph, rather than a second execution method.
func CheckCalibrator(t *testing.T, node core.Primitive, capacity int) {
	t.Helper()
	reference := referencePaceNewCalibrator(referencePaceCalibratorConfig{Window: capacity})
	for index, sample := range []float64{10, 20, 30, 15, 40, 50, 1, 5, 99, 4} {
		prior := reference.Count()
		want, err := reference.Measure(sample)
		if err != nil {
			t.Fatal(err)
		}
		out := Drain(t, node, Values(sample))
		Sound(t, node)
		if len(out) != 1 {
			t.Fatalf("sample %d: outputs=%d", index, len(out))
		}
		f := Fields(t, out[0])
		EqualNumber(t, Number(t, f, "value"), want.Value)
		EqualNumber(t, Number(t, f, "prior_count"), float64(prior))
		if core.To[bool](f["ready"]) != want.Ready {
			t.Fatal("prior readiness")
		}
	}
}

// CheckCalibratorPoison checks admission without changing the prior window.
func CheckCalibratorPoison(t *testing.T, makeNode func() core.Primitive) {
	t.Helper()
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		node := makeNode()
		Drain(t, node, Values(10.0))
		if out := Drain(t, node, Values(bad)); len(out) != 0 || node.Error() == nil {
			t.Fatal("poison admitted")
		}
		out := Drain(t, node, Values(5.0))
		if len(out) != 1 {
			t.Fatal("failed input damaged delivery")
		}
		f := Fields(t, out[0])
		EqualNumber(t, Number(t, f, "prior_count"), 1)
		EqualNumber(t, Number(t, f, "value"), 1)
	}
}
