package collection_test

import (
	"errors"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"math"
	"reflect"
	"testing"
)

func TestSetNext(t *testing.T) {
	node := collection.NewSet[float64](tests.Values(1.0), tests.Values(7.0))
	original := []float64{2, 3, 4}
	output := tests.Drain(t, node, tests.Values(original))
	tests.Sound(t, node)
	tests.EqualNumber(t, output[0].([]float64)[1], 7)
	tests.EqualNumber(t, original[1], 3)
}

func TestSetRejectsInvalidIndex(t *testing.T) {
	for _, index := range []float64{-1, 0.5, 3, math.NaN(), math.Inf(1)} {
		node := collection.NewSet[float64](tests.Values(index), tests.Values(8.0))
		tests.Drain(t, node, tests.Values([]float64{1, 2, 3}))
		if !errors.Is(node.Error(), core.ErrShape) {
			t.Fatalf("invalid index %v accepted", index)
		}
	}
}

func TestSetConfiguredConnections(t *testing.T) {
	index := store.NewRetained(core.From(0.0))
	value := store.NewRetained(core.From("first"))
	node := collection.NewSet[string](index, value)
	out := tests.Drain(t, node, tests.Values([]string{"a", "b"}))
	tests.Sound(t, node)
	if !reflect.DeepEqual(out[0], []string{"first", "b"}) {
		t.Fatal(out)
	}
	tests.Drain(t, index, tests.Values(1.0))
	tests.Drain(t, value, tests.Values("second"))
	out = tests.Drain(t, node, tests.Values([]string{"a", "b"}))
	tests.Sound(t, node)
	if !reflect.DeepEqual(out[0], []string{"a", "second"}) {
		t.Fatal("configured connection disappeared", out)
	}
}
