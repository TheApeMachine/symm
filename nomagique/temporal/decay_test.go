package temporal

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
	"testing"
)

func TestDecayComposition(t *testing.T) {
	node := NewDecay(nil, nil)
	tests.EqualNumber(t, tests.Drain(t, node, transport.NewIO(core.From(10.0)))[0], 0)
	node = NewDecay(store.NewConstant(core.From(0.25)), nil)
	tests.EqualNumber(t, tests.Drain(t, node, transport.NewIO(core.From(10.0)))[0], 7.5)
	node = NewDecay(store.NewConstant(core.From(math.Ln2)), transport.NewPipe(calculus.NewNegate(transport.NewIO(core.From(0.0))), calculus.NewExp(transport.NewIO(core.From(0.0)))))
	tests.EqualNumber(t, tests.Drain(t, node, transport.NewIO(core.From(10.0)))[0], 5)
}
