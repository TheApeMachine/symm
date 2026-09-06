package tests

import (
	"github.com/theapemachine/symm/nomagique/core"
	"math/rand"
	"testing"
)

func CheckLocalRegression(t *testing.T, node core.Primitive) {
	t.Helper()
	reference := referenceLocalRegression{}
	rng := rand.New(rand.NewSource(113))
	at := int64(1700000000000000000)
	for step := 0; step < 100; step++ {
		if step > 2 {
			at += int64(1000000 + rng.Intn(1000000000))
		}
		value := float64(step)*0.1 + rng.NormFloat64()
		reference.Step(value, at, 0)
		out := Drain(t, node, Values(Record(map[string]any{"at": at, "value": value})))
		Sound(t, node)
		f := Fields(t, out[0])
		slope, ok := reference.Slope()
		EqualNumber(t, Number(t, f, "slope"), slope)
		if core.To[bool](f["slope_defined"]) != ok {
			t.Fatal("slope domain differs")
		}
		snr, ok := reference.SNR()
		EqualNumber(t, Number(t, f, "snr"), snr)
		if core.To[bool](f["snr_defined"]) != ok {
			t.Fatal("snr domain differs")
		}
	}
}
