package nomagique

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Number composes Primitives into a single numeric result by threading each
one's output into the next one's input. It is the canonical way to express a
calculation in this system: rather than a type that knows how to compute a
decay or a running average, a pipeline of small Primitives that each know one
operation.

The fold starts from nil, which is not a special case but the base of the
composition. The first Primitive, having been offered nothing, answers with
its own configured value, and that becomes the input to the second. So a
pipeline needs no separate notion of a source.

Each call advances every Primitive in the composition exactly one step.
Stateful Primitives keep what they accumulated, which makes Number a step
rather than an evaluation: calling it twice is two ticks of the same
pipeline, not the same calculation performed twice.
*/
func Number(primitives ...core.Primitive) (out core.Primitive) {
	for _, primitive := range primitives {
		out = primitive.Next(out)

		if out.Error() != nil {
			return
		}
	}

	return
}
