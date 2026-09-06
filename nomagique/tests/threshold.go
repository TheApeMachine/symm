package tests

import (
	"github.com/theapemachine/symm/nomagique/core"
	"math"
	"testing"
)

// CheckThreshold checks the source's dispersion multiplier policies. Its "MAD"
// branch used a normal consistency multiplier on SD; it did not calculate MAD.
func CheckThreshold(t *testing.T, node core.Primitive, policy string) {
	t.Helper()
	count, mean, m2 := 0.0, 0.0, 0.0
	for _, x := range []float64{2, 2, 3, 6, 1, 9, 8, 0, 4} {
		count++
		delta := x - mean
		mean += delta / count
		m2 += delta * (x - mean)
		want := 1.0
		if count > 1 && m2 > 0 {
			coefficient := 1.0
			switch policy {
			case "predictive":
				coefficient = math.Sqrt(1 + 1/count)
			case "normal":
				coefficient = 1.482602218505602
			case "chebyshev":
				coefficient = math.Sqrt(count)
			}
			want = math.Sqrt(m2/(count-1)) * coefficient
		}
		out := Drain(t, node, Values(x))
		Sound(t, node)
		if len(out) != 1 {
			t.Fatal("threshold output")
		}
		EqualNumber(t, out[0], want)
	}
}
