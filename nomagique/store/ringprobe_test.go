package store_test

import (
	container "container/ring"
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestRingProbe(t *testing.T) {
	held := container.New(6)

	for _, value := range []core.Primitive{
		core.From(1.0), core.From(2.0), nil,
		core.From(3.0), core.From(4.0), nil,
	} {
		held.Value = value
		held = held.Next()
	}
	replay := store.NewRing(store.NewRetained(core.From(held)))

	for run := range 5 {
		var got []float64

		for value := replay.Next(nil); value != nil; value = replay.Next(nil) {
			got = append(got, value.Read().(float64))
		}
		t.Logf("run %d: %v err=%v", run, got, replay.Error())
	}
}
