package core_test

import (
	"errors"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestProtoIsInert(t *testing.T) {
	value := core.From(7.0)
	if value.Next(nil) != nil || value.Next(core.From(8)) != nil {
		t.Fatal("Proto emitted")
	}
	tests.EqualNumber(t, core.To[float64](value), 7)
}
func TestYieldUsesDelivery(t *testing.T) {
	value := core.From(2.0)
	add := arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0)))
	tests.EqualNumber(t, tests.Drain(t, add, value)[0], 0)
	for range 3 {
		tests.EqualNumber(t, tests.Drain(t, add, transport.NewIO(value))[0], 2)
	}
}
func TestYieldErrorsTravel(t *testing.T) {
	add := arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0)))
	tests.Drain(t, add, transport.NewIO(core.From("bad")))
	if !errors.Is(add.Error(), core.ErrWrongType) {
		t.Fatal(add.Error())
	}
	left := core.From("bad seed")
	add = arithmetic.NewAdd[float64](transport.NewIO(left))
	tests.Drain(t, add, transport.NewIO(core.From(8.0)))
	if !errors.Is(add.Error(), core.ErrConversion) {
		t.Fatal("seed error lost")
	}
}
