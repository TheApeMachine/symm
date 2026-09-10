package associative

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Agent is a associative learner that uses action classification
and reinforcement to adjust its behavior.
*/
type Agent struct {
	*runtime.System
	current core.Primitive
}

/*
NewAgent creates a new Agent with the given System.
*/
func NewAgent() *Agent {
	return &Agent{}
}

func (agent *Agent) Next(in core.Primitive) core.Primitive {
	result := core.Yield(
		agent.current,
		in,
		func(held, value float64) float64 {
			return value
		},
		agent,
	)

	return result
}

func (agent *Agent) Read() any {
	return core.To[any](agent.current)
}
