package tests

import (
	"github.com/theapemachine/symm/nomagique/core"
	"math/rand"
	"testing"
)

func CheckJoint(t *testing.T, node core.Primitive) {
	t.Helper()
	rng := rand.New(rand.NewSource(13))
	reference := referenceJointEstimator{}
	for step := 0; step < 120; step++ {
		values := [3]float64{rng.NormFloat64(), rng.NormFloat64() * 2, 0}
		reference.Step(values, float64(step), 0)
		out := Drain(t, node, Values(Record(map[string]any{"values": values[:], "at": int64(step) * 1e9})))
		Sound(t, node)
		f := Fields(t, out[0])
		channels := core.To[[]core.Primitive](f["channels"])
		if len(channels) != 3 {
			t.Fatal("lost coordinate")
		}
		for i, c := range channels {
			cf := Fields(t, c.Read())
			EqualNumber(t, Number(t, cf, "baseline"), reference.Baseline(i))
			EqualNumber(t, Number(t, cf, "ratio"), reference.Ratio(i))
			EqualNumber(t, Number(t, cf, "residual"), reference.Residual(i))
			noise, ok := reference.Noise(i)
			EqualNumber(t, Number(t, cf, "noise"), noise)
			if core.To[bool](cf["noise_defined"]) != ok {
				t.Fatal("noise support changed")
			}
			z, _ := reference.ZScore(i)
			EqualNumber(t, Number(t, cf, "zscore"), z)
		}
		snr, ok := reference.SNR()
		EqualNumber(t, Number(t, f, "snr"), snr)
		if core.To[bool](f["snr_defined"]) != ok {
			t.Fatal("joint support changed")
		}
	}
}
