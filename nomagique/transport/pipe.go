package transport

import "github.com/theapemachine/symm/nomagique/core"

// Pipe moves each complete stage run to the next stage. Intermediate runs are
// buffered as opaque Primitives, not decoded values. Completing a stage before
// entering the next also permits a shared store to occur twice in the graph.
type Pipe struct {
	core.PrimitiveError
	stages  []core.Primitive
	output  core.Primitive
	current core.Primitive
}

func NewPipe(stages ...core.Primitive) *Pipe {
	return &Pipe{stages: append([]core.Primitive(nil), stages...)}
}
func (pipe *Pipe) Next(in core.Primitive) core.Primitive {
	if pipe.output == nil {
		pipe.output = in
		for _, stage := range pipe.stages {
			values := []core.Primitive{}
			core.Yield(
				NewIO(core.From(0)),
				NewApply(stage, pipe.output),
				func(held int, value core.Primitive) int {
					values = append(values, value)
					return held
				},
				pipe,
			)
			pipe.output = NewIO(values...)
		}
	}
	if pipe.output == nil {
		return nil
	}
	value := pipe.output.Next(nil)
	pipe.Error(pipe.output.Error())
	if value == nil {
		pipe.output = nil
	} else {
		pipe.current = value
	}
	return value
}
func (pipe *Pipe) Read() any { return core.To[any](pipe.current) }
