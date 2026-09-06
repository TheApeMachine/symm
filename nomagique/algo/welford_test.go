package algo

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestWelfordRecurrence(t *testing.T) {
	node := NewWelford()
	input := transport.NewIO(core.From(1.0), core.From(2.0), core.From(3.0), core.From(4.0))
	out := tests.Drain(t, node, input)
	if node.Error() != nil {
		t.Fatal(node.Error())
	}
	if len(out) != 4 {
		t.Fatal(out)
	}
	for i, value := range out {
		m := value.(map[string]core.Primitive)
		tests.EqualNumber(t, core.To[float64](m["count"]), float64(i+1))
		tests.EqualNumber(t, core.To[float64](m["prior_count"]), float64(i))
		tests.EqualNumber(t, core.To[float64](m["mean"]), float64(i+2)/2)
	}
	last := out[3].(map[string]core.Primitive)
	tests.EqualNumber(t, core.To[float64](last["variance"]), 5.0/3)
	more := tests.Drain(t, node, transport.NewIO(core.From(5.0)))[0].(map[string]core.Primitive)
	tests.EqualNumber(t, core.To[float64](more["count"]), 5)
	tests.EqualNumber(t, core.To[float64](more["mean"]), 3)
	tests.EqualNumber(t, core.To[float64](out[0].(map[string]core.Primitive)["variance"]), math.NaN())
}
