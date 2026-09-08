package logic

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Gate routes a captured input run through the selected branch. Its one branch is
the selection operation itself; neither the predicate nor either branch needs to
know why it was selected.

A Gate delivers many runs over its lifetime, so its plumbing is built once and
rewound per run rather than reconstructed. Constructing it per run was the
single largest source of allocation in the graph. The predicate and the chosen
branch read the captured input through separate streams because each stream
owns one cursor.
*/
type Gate struct {
	core.PrimitiveError
	predicate, pass, fail core.Primitive
	output, current       core.Primitive

	// Seeds for the two folds below. Their values never change, so they are
	// carried rather than rebuilt.
	captureSeed, decisionSeed core.Primitive
	capture, decision         *transport.IO

	// One stream per reader of the captured input, and one bound call per
	// branch, all rewound at the start of each run.
	predicateArguments, branchArguments *transport.IO
	predicateCall                       *transport.Apply
	passCall, failCall                  *transport.Apply

	arguments []core.Primitive
}

func NewGate(predicate, pass, fail core.Primitive) *Gate {
	gate := &Gate{
		predicate: predicate, pass: pass, fail: fail,
		captureSeed:        core.From(0),
		decisionSeed:       core.From(false),
		capture:            transport.NewIO(),
		decision:           transport.NewIO(),
		predicateArguments: transport.NewIO(),
		branchArguments:    transport.NewIO(),
	}

	gate.predicateCall = transport.NewApply(predicate, gate.predicateArguments)
	gate.passCall = transport.NewApply(pass, gate.branchArguments)
	gate.failCall = transport.NewApply(fail, gate.branchArguments)

	return gate
}

func (gate *Gate) Next(in core.Primitive) core.Primitive {
	if gate.output == nil {
		selected := false
		observed := 0

		gate.arguments = gate.arguments[:0]
		gate.capture.Reset(gate.captureSeed)
		core.Yield(gate.capture, in, func(n int, v core.Primitive) int {
			gate.arguments = append(gate.arguments, v)

			return n
		}, gate)

		gate.predicateArguments.Reset(gate.arguments...)
		gate.decision.Reset(gate.decisionSeed)
		core.Yield(
			gate.decision,
			gate.predicateCall,
			func(_, v bool) bool { selected = v; observed++; return v },
			gate,
		)

		if observed != 1 {
			gate.Error(core.ErrShape)

			return nil
		}

		gate.branchArguments.Reset(gate.arguments...)
		gate.output = gate.failCall

		if selected {
			gate.output = gate.passCall
		}
	}

	value := gate.output.Next(nil)
	gate.Error(gate.output.Error())

	if value == nil {
		gate.output = nil

		return nil
	}

	gate.current = value

	return value
}

func (gate *Gate) Read() any { return core.To[any](gate.current) }
