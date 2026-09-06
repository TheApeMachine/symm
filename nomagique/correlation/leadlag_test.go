package correlation_test

import (
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"math"
	"testing"
)

func TestLeadLagNext(t *testing.T) {
	times := make([]int64, 16)
	left, right := make([]float64, 16), make([]float64, 16)
	for i := range times {
		times[i] = int64(i) * 1e9
		left[i] = math.Exp(math.Sin(float64(i)))
	}
	for i := range times {
		right[i] = left[max(0, i-2)]
	}
	node := correlation.NewLeadLag(algo.NewHayashiYoshida())
	var originalProfile []core.Primitive
	for range 2 {
		out := tests.Drain(t, node, tests.Observation(tests.Path(times, left), tests.Path(times, right)))
		tests.Sound(t, node)
		if len(out) != 1 {
			t.Fatal(len(out))
		}
		f := tests.Fields(t, out[0])
		tests.EqualNumber(t, tests.Number(t, f, "x"), 2)
		tests.EqualNumber(t, tests.Number(t, f, "spacing"), 1e9)
		tests.EqualNumber(t, tests.Number(t, f, "span"), 14)
		tests.EqualNumber(t, tests.Number(t, f, "lag_index"), 2)
		if !core.To[bool](f["defined"]) {
			t.Fatal("defined peak lost")
		}
		profile := core.To[[]core.Primitive](f["profile"])
		if len(profile) != 29 {
			t.Fatalf("profile lost: %d", len(profile))
		}
		selected := core.To[map[string]core.Primitive](profile[int(tests.Number(t, f, "index"))])
		tests.EqualNumber(t, tests.Number(t, selected, "support"), tests.Number(t, f, "support"))
		if originalProfile == nil {
			originalProfile = profile
		}
		if tests.Number(t, f, "support") <= 0 {
			t.Fatal("winning support lost")
		}
	}
	out := tests.Drain(t, node, tests.Observation(nil, nil))
	tests.Sound(t, node)
	if len(core.To[[]core.Primitive](tests.Fields(t, out[0])["profile"])) != 0 {
		t.Fatal("empty input retained old profile")
	}
	tests.EqualNumber(t, core.To[float64](core.To[map[string]core.Primitive](originalProfile[0])["x"]), -14)
	if core.To[bool](tests.Fields(t, out[0])["defined"]) {
		t.Fatal("empty input returned stale peak")
	}
}

func TestLagShapeUsesSelectedIndex(t *testing.T) {
	points := make([]core.Primitive, 5)
	for i, y := range []float64{.1, .2, 1, .8, .2} {
		points[i] = tests.Record(map[string]any{"x": float64(i - 2), "y": y, "defined": true})
	}
	profile := store.NewRetained(core.From(points))
	selected := store.NewRetained(tests.Record(map[string]any{"index": 3.0}))
	node := correlation.NewLagShape(profile, selected)
	out := tests.Drain(t, node, tests.Values(tests.Record(map[string]any{
		"index": 3.0, "span": 2.0, "spacing": 1e9, "y": .8})))
	tests.Sound(t, node)
	f := tests.Fields(t, out[0])
	tests.EqualNumber(t, tests.Number(t, f, "prominence"), .2)
	tests.EqualNumber(t, tests.Number(t, f, "curvature"), .4)
}
