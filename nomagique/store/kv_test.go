package store_test

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestKVFreshAndRetainedConnections(t *testing.T) {
	empty := map[string]core.Primitive{}
	fresh := store.NewKV[string](transport.NewIO(core.From(empty)))
	memory := store.NewRetained(core.From(empty))
	persistent := transport.NewPipe(store.NewKV[string](memory), memory)
	for _, operation := range []core.Primitive{fresh, persistent} {
		first := tests.Drain(t, operation, transport.NewIO(core.From(map[string]core.Primitive{"a": core.From(1.0)})))[0].(map[string]core.Primitive)
		second := tests.Drain(t, operation, transport.NewIO(core.From(map[string]core.Primitive{"b": core.From(2.0)})))[0].(map[string]core.Primitive)
		if _, found := second["a"]; found != (operation == persistent) {
			t.Fatal("retention is not supplied by topology")
		}
		if len(first) != 1 {
			t.Fatal("previous output was mutated")
		}
	}
	if len(empty) != 0 {
		t.Fatal("configured input map mutated")
	}
}
