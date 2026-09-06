package transport_test

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestPipeAndMap(t *testing.T) {
	pipe := transport.NewPipe(
		transport.NewMap(calculus.NewSquare(transport.NewIO(core.From(0.0)))),
		arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))),
	)
	for range 3 {
		tests.EqualNumber(t, tests.Drain(t, pipe, transport.NewIO(core.From(2.0), core.From(3.0)))[0], 13)
	}
}
func TestFanIsOpaqueTransport(t *testing.T) {
	fan := transport.NewFan(transport.NewPipe(), transport.NewIO(
		arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))),
		transport.NewMapReduce(calculus.NewSquare(transport.NewIO(core.From(0.0))), arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0)))),
	))
	got := tests.Drain(t, fan, transport.NewIO(core.From(2.0), core.From(3.0)))
	if len(got) != 2 {
		t.Fatal(got)
	}
	tests.EqualNumber(t, got[0], 5)
	tests.EqualNumber(t, got[1], 13)
}
