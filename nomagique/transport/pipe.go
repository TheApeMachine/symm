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
	buffers [2]IO
}

func NewPipe(stages ...core.Primitive) *Pipe {
	return &Pipe{stages: append([]core.Primitive(nil), stages...)}
}
func (pipe *Pipe) Next(in core.Primitive) core.Primitive {
	if pipe.output == nil {
		pipe.output = in
		for index, stage := range pipe.stages {
			// Adjacent stages need separate buffers until the input is drained.
			buffer := &pipe.buffers[index%len(pipe.buffers)]
			buffer.reset()
			pipe.Error(buffer.appendRun(stage, pipe.output))
			pipe.output = buffer
		}
	}

	if pipe.output == nil {
		return nil
	}
	value := pipe.output.Next(nil)
	pipe.Error(pipe.output.Error())

	if value == nil {
		pipe.output = nil
		for index := range pipe.buffers {
			pipe.buffers[index].reset()
		}

		return nil
	}

	pipe.current = value
	return value
}
func (pipe *Pipe) Read() any { return core.To[any](pipe.current) }
