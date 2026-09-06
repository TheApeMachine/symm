package tests

import (
	"github.com/theapemachine/symm/nomagique/core"
	"math"
	"testing"
)

// CheckTarget keeps the source target formulas at one independent test boundary.
func CheckTarget(t *testing.T, name string, node core.Primitive, band float64) {
	t.Helper()
	for _, pair := range [][2]float64{{2, 1}, {-1, 2}, {0, 2}, {2, 2}, {2.5, 2}, {-2, -3}} {
		current, past := pair[0], pair[1]
		wanted := current - past
		switch name {
		case "identity":
			wanted = current
		case "ratio":
			wanted = current/past - 1
		case "binary":
			wanted = 0
			if current > past {
				wanted = 1
			}
		case "directional":
			wanted = 0
			if math.Abs(current-past) > band {
				wanted = math.Copysign(1, current-past)
			}
		}
		out := Drain(t, node, Values(Record(map[string]any{"current": current, "past": past})))
		Sound(t, node)
		if len(out) != 1 {
			t.Fatal("target did not yield exactly once")
		}
		EqualNumber(t, out[0], wanted)
	}
}
