package logic_test

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestLogicalPrimitiveRuns(t *testing.T) {
	cases := []struct {
		op     core.Primitive
		values []bool
		want   bool
	}{
		{logic.NewAnd(transport.NewIO(core.From(true))), []bool{true, false}, false},
		{logic.NewOr(transport.NewIO(core.From(false))), []bool{false, true}, true},
		{logic.NewNot(transport.NewIO(core.From(false))), []bool{false}, true},
	}
	for _, c := range cases {
		members := []core.Primitive{}
		for _, v := range c.values {
			members = append(members, core.From(v))
		}
		out := tests.Drain(t, c.op, transport.NewIO(members...))
		if out[0].(bool) != c.want {
			t.Fatal(out)
		}
	}
}
func TestComparisons(t *testing.T) {
	cases := []struct {
		op     core.Primitive
		values []float64
		want   bool
	}{
		{logic.NewLess[float64](), []float64{1, 2}, true},
		{logic.NewLessEqual[float64](), []float64{2, 2}, true},
		{logic.NewGreater[float64](), []float64{1, 2}, false},
		{logic.NewEqual[float64](), []float64{2, 2}, true},
	}
	for _, c := range cases {
		out := tests.Drain(t, c.op, transport.NewIO(core.From(c.values)))
		if out[0].(bool) != c.want {
			t.Fatal(out)
		}
	}
}
