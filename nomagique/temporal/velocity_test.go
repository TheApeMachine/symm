package temporal_test

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
	"time"
)

func TestVelocityNext(t *testing.T) {
	node := temporal.NewVelocity(store.NewGet("value"), store.NewGet("at"))
	origin := time.Unix(1700000000, 0).UnixNano()
	inputs := []core.Primitive{
		tests.Record(map[string]any{"value": 10.0, "at": origin}),
		tests.Record(map[string]any{"value": 13.0, "at": origin + int64(1000000000)}),
		tests.Record(map[string]any{"value": 16.0, "at": origin + int64(1000000000)}),
		tests.Record(map[string]any{"value": 17.0, "at": origin + int64(1000000001)}),
	}
	output := tests.Drain(t, node, tests.Values(inputs...))
	tests.Sound(t, node)
	if len(output) != 4 {
		t.Fatal(output)
	}
	for index, want := range []float64{0, 3, 0, 1e9} {
		tests.EqualNumber(t, tests.Number(t, tests.Fields(t, output[index]), "rate"), want)
	}
	next := tests.Drain(t, node, tests.Values(tests.Record(map[string]any{"value": 18.0, "at": origin + int64(2000000001)})))
	tests.EqualNumber(t, tests.Number(t, tests.Fields(t, next[0]), "rate"), 1)
}
