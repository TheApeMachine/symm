package tests

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Run hands over several Primitives in one delivery run, and is how a case
puts a Primitive under the obligation the contract actually imposes: a
neighbour may yield once or many times, and a Primitive that quietly assumes
the first is only correct by accident.

It is here rather than in each package because every Primitive that consumes
input needs it, and a test fixture that is written out again in each file is
the same duplication the algebra refuses everywhere else.

The run belongs to the caller, so a different caller is handed the values
from the beginning rather than the remains of somebody else's pass.
*/
type Run struct {
	core.PrimitiveError
	values []core.Primitive
	index  int
	caller core.Primitive
	open   bool
}

/*
NewRun configures the Primitives handed over, in order.
*/
func NewRun(values ...core.Primitive) *Run {
	return &Run{
		values: values,
	}
}

/*
Next hands over the next Primitive of the current run, and nil once the run
is spent.
*/
func (run *Run) Next(in core.Primitive) core.Primitive {
	if !run.open || in != run.caller {
		run.index, run.caller, run.open = 0, in, true
	}

	if run.index >= len(run.values) {
		run.open = false

		return nil
	}

	run.index++

	return run.values[run.index-1]
}

/*
Read surfaces the run being handed over for the boundary.
*/
func (run *Run) Read() any {
	return run.values
}
