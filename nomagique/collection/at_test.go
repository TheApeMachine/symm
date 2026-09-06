package collection_test

import (
	"errors"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
	"testing"
)

func TestAtNext(t *testing.T) {
	for _, index := range []float64{0, 1, 2} {
		operation := collection.NewAt[float64](transport.NewIO(core.From(index)))
		out := tests.Drain(t, operation, transport.NewIO(core.From([]float64{3, 4, 5})))
		tests.EqualNumber(t, out[0], index+3)
	}
	for _, index := range []float64{-1, .5, 3, math.NaN(), math.Inf(1)} {
		operation := collection.NewAt[float64](transport.NewIO(core.From(index)))
		out := operation.Next(transport.NewIO(core.From([]float64{3, 4, 5})))
		if !errors.Is(operation.Error(), core.ErrShape) || out == nil || !errors.Is(out.Error(), core.ErrShape) {
			t.Fatalf("index %v not rejected", index)
		}
	}
}
