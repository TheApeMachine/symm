package logic

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

// Gate routes a captured input run through the selected branch. Its one branch
// is the selection operation itself; neither the predicate nor either branch
// needs to know why it was selected.
type Gate struct {
	core.PrimitiveError
	predicate, pass, fail core.Primitive
	output, current       core.Primitive
}

func NewGate(predicate, pass, fail core.Primitive) *Gate {
	return &Gate{predicate: predicate, pass: pass, fail: fail}
}
func (gate *Gate) Next(in core.Primitive) core.Primitive {
	if gate.output == nil {
		input := []core.Primitive{}
		selected := false
		observed := 0
		core.Yield(transport.NewIO(core.From(0)), in, func(n int, v core.Primitive) int { input = append(input, v); return n }, gate)
		core.Yield(
			transport.NewIO(core.From(false)),
			transport.NewApply(gate.predicate, transport.NewIO(input...)),
			func(_, v bool) bool { selected = v; observed++; return v },
			gate,
		)
		if observed != 1 {
			gate.Error(core.ErrShape)
			return nil
		}
		branch := gate.fail
		if selected {
			branch = gate.pass
		}
		gate.output = transport.NewApply(branch, transport.NewIO(input...))
	}
	value := gate.output.Next(nil)
	gate.Error(gate.output.Error())
	if value == nil {
		gate.output = nil
	} else {
		gate.current = value
	}
	return value
}
func (gate *Gate) Read() any { return core.To[any](gate.current) }
