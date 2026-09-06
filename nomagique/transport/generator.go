package transport

import (
	"github.com/theapemachine/symm/nomagique/core"
	"iter"
)

// Generator is the boundary from a Go iterator into Primitive delivery. It is
// one-shot: exhaustion never restarts a stateful external source.
type Generator[T any] struct {
	core.PrimitiveError
	source  iter.Seq[T]
	pull    func() (T, bool)
	stop    func()
	current core.Primitive
	done    bool
}

func NewGenerator[T any](source iter.Seq[T]) *Generator[T] {
	generator := &Generator[T]{source: source}
	if source == nil {
		generator.Error(core.ErrNotHeld)
		generator.done = true
	}
	return generator
}
func (generator *Generator[T]) Next(core.Primitive) core.Primitive {
	if generator.done {
		return nil
	}
	if generator.pull == nil {
		generator.pull, generator.stop = iter.Pull(generator.source)
	}
	value, ok := generator.pull()
	if !ok {
		generator.stop()
		generator.done = true
		return nil
	}
	generator.current = core.From(value)
	return generator.current
}
func (generator *Generator[T]) Read() any { return core.To[any](generator.current) }
