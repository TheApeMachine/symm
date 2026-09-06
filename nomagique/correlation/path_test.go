package correlation_test

import (
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
)

func TestPathNext(t *testing.T) {
	path := correlation.NewPath()
	var earliest map[string]core.Primitive
	for i, c := range []struct {
		at                 int64
		v, count           float64
		accepted, restated bool
	}{
		{10, 100, 1, true, false}, {12, 105, 2, true, false}, {12, 106, 2, true, true},
		{11, 999, 2, false, false}, {13, 107, 3, true, false},
	} {
		out := tests.Drain(t, path, tests.Values(tests.Record(map[string]any{"at": c.at, "value": c.v})))
		tests.Sound(t, path)
		if len(out) != 1 {
			t.Fatal(out)
		}
		f := tests.Fields(t, out[0])
		tests.EqualNumber(t, tests.Number(t, f, "count"), c.count)
		if core.To[bool](f["accepted"]) != c.accepted || core.To[bool](f["restated"]) != c.restated {
			t.Fatal("status", i)
		}
		obs := core.To[[]core.Primitive](f["observations"])
		last := core.To[map[string]core.Primitive](obs[len(obs)-1])
		wanted := c.v
		if !c.accepted {
			wanted = 106
		}
		tests.EqualNumber(t, tests.Number(t, last, "value"), wanted)
		if i == 1 {
			earliest = f
		}
	}
	retained := core.To[[]core.Primitive](earliest["observations"])
	tests.EqualNumber(t, tests.Number(t, core.To[map[string]core.Primitive](retained[1]), "value"), 105)
}
func TestPathRetentionIsConfiguration(t *testing.T) {
	path := correlation.NewPath(collection.NewTail[core.Primitive](store.NewConstant(core.From(2))))
	source := tests.Values(
		tests.Record(map[string]any{"at": int64(1), "value": 1.0}),
		tests.Record(map[string]any{"at": int64(2), "value": 2.0}),
		tests.Record(map[string]any{"at": int64(3), "value": 3.0}))
	out := tests.Drain(t, path, source)
	tests.Sound(t, path)
	if len(out) != 3 {
		t.Fatal(out)
	}
	last := tests.Fields(t, out[2])
	tests.EqualNumber(t, tests.Number(t, last, "count"), 2)
	if core.To[int64](last["from"]) != 2 || core.To[int64](last["to"]) != 3 {
		t.Fatal("retained span")
	}
}
