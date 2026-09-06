package tests

import (
	"github.com/theapemachine/symm/nomagique/core"
	"math/rand"
	"testing"
)

func CheckPace(t *testing.T, node core.Primitive) {
	t.Helper()
	reference := referenceNewPaceController(referencePaceConfig{InitialAlpha: 0.03, MinAlpha: 0.005, MaxAlpha: 0.15, Gain: 0.1, Band: 0.2, Window: 8})
	rng := rand.New(rand.NewSource(86))
	for i := 0; i < 200; i++ {
		errorValue := rng.Float64() + float64(i/50)
		expected, err := reference.Measure(errorValue)
		if err != nil {
			t.Fatal(err)
		}
		out := Drain(t, node, Values(errorValue))
		Sound(t, node)
		f := Fields(t, out[0])
		EqualNumber(t, Number(t, f, "alpha"), expected.Alpha)
		EqualNumber(t, Number(t, f, "rank"), expected.Rank)
		EqualNumber(t, Number(t, f, "count"), float64(expected.Count))
		if core.To[bool](f["ready"]) != expected.Ready {
			t.Fatal("pace readiness differs")
		}
	}
}
