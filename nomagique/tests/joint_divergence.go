package tests

import (
	"errors"
	"github.com/theapemachine/symm/nomagique/core"
	"testing"
)

func CheckJointDivergence(t *testing.T, node core.Primitive) {
	t.Helper()
	var fields map[string]core.Primitive
	for index := range 4 {
		out := Drain(t, node, Values(Record(map[string]any{"values": []float64{float64(index), -2 * float64(index)}, "at": int64(index) * 1e9})))
		Sound(t, node)
		if len(out) != 1 {
			t.Fatal("one divergence record expected")
		}
		fields = Fields(t, out[0])
	}
	channels := core.To[[]core.Primitive](fields["channels"])
	velocities := core.To[[]core.Primitive](fields["velocities"])
	if len(channels) != 2 || len(velocities) != 2 {
		t.Fatal("channel cardinality")
	}
	for index, scale := range []float64{1, -2} {
		channel := core.To[map[string]core.Primitive](channels[index])
		velocity := core.To[map[string]core.Primitive](velocities[index])
		EqualNumber(t, Number(t, channel, "residual"), 2*scale)
		// OLS of residuals [0,1,1.5,2] over seconds [0,1,2,3].
		EqualNumber(t, Number(t, velocity, "slope"), (3.25/5)*scale)
		if !core.To[bool](velocity["slope_defined"]) {
			t.Fatal("undefined velocity")
		}
	}
}

func CheckJointRegression(t *testing.T, node, left core.Primitive) {
	t.Helper()
	for index, at := range []int64{2e9, 1e9} {
		out := Drain(t, node, Values(Record(map[string]any{"values": []float64{1, -2}, "at": at})))
		if len(out) != 1-index {
			t.Fatal("regression not rejected")
		}
		EqualNumber(t, Number(t, Fields(t, left.Read()), "count"), 1)
	}
	if !errors.Is(node.Error(), core.ErrDomain) {
		t.Fatal("missing domain error")
	}
}
