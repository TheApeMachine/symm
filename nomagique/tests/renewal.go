package tests

import (
	"errors"
	"github.com/theapemachine/symm/nomagique/core"
	"math"
	"testing"
)

func CheckRenewalRate(t *testing.T, node core.Primitive) {
	t.Helper()
	for _, row := range []struct {
		at                                        int64
		increment, sample, rate, change, maturity float64
		closed                                    bool
	}{
		{0, 2, 100, 0, 0, 0, false}, {1e9, 2, 100, 4, 0, .5, true},
		{2e9, 1, 105, 4, 0, .5, false}, {3e9, 3, 110, 2, math.Log(1.1), 2.0 / 3, true},
	} {
		out := Drain(t, node, Values(Record(map[string]any{"increment": row.increment, "sample": row.sample, "at": row.at})))
		Sound(t, node)
		if len(out) != 1 {
			t.Fatal("one renewal record expected")
		}
		f := Fields(t, out[0])
		if core.To[bool](f["closed"]) != row.closed {
			t.Fatal("closed")
		}
		EqualNumber(t, Number(t, f, "rate"), row.rate)
		EqualNumber(t, Number(t, f, "change"), row.change)
		EqualNumber(t, Number(t, f, "maturity"), row.maturity)
	}
}

func CheckRenewalMissing(t *testing.T, node core.Primitive) {
	t.Helper()
	Drain(t, node, Values(Record(map[string]any{"increment": 2.0, "sample": 100.0, "at": int64(0)})))
	out := Drain(t, node, Values(Record(map[string]any{"sample": 100.0, "at": int64(1e9)})))
	if len(out) != 0 || !errors.Is(node.Error(), core.ErrShape) {
		t.Fatal("missing increment reused")
	}
}
