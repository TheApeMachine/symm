package store_test

import (
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
)

func TestHasNext(t *testing.T) {
	node := store.NewHas("x")
	for _, example := range []struct {
		fields map[string]any
		want   bool
	}{{map[string]any{"x": 0.0}, true}, {map[string]any{}, false}} {
		output := tests.Drain(t, node, tests.Values(tests.Record(example.fields)))
		tests.Sound(t, node)
		if len(output) != 1 || output[0] != example.want {
			t.Fatal(output)
		}
	}
}
