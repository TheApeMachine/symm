package store_test

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestRetainedConnection(t *testing.T) {
	left := store.NewRetained(core.From(10.0))
	subtraction := arithmetic.NewSubtract[float64](left)
	tests.EqualNumber(t, tests.Drain(t, subtraction, transport.NewIO(core.From(3.0)))[0], 7)
	tests.Drain(t, left, transport.NewIO(core.From(20.0)))
	tests.EqualNumber(t, tests.Drain(t, subtraction, transport.NewIO(core.From(5.0), core.From(2.0)))[0], 13)
	tests.EqualNumber(t, tests.Drain(t, subtraction, nil)[0], 20)
	tests.EqualNumber(t, core.To[float64](left), 20)
}
