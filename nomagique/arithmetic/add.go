package arithmetic

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Add owns one field operation. Configuration supplies the value a run starts
from; recurrence and delivery remain separate Primitives.

Add is itself Primitive[T, U]: what it hands over is its own accumulated
value, of type U every time. T is only the type of what arrives on the
input run, which may differ from U (a narrower reading widened into a wider
running total).
*/
type Add[T, U core.Numeric] struct {
	core.Base[T, U]
}

/*
NewAdd creates a new Add primitive with the given initial value. It initializes
the internal state and prepares the primitive for processing incoming values.
*/
func NewAdd[T, U core.Numeric](current U) *Add[T, U] {
	op := &Add[T, U]{}
	op.Carrier(current)
	return op
}

/*
Next folds the incoming run into the value it was configured with and hands
that value over after every arrival. What it hands over is itself.
*/
func (op *Add[T, U]) Next(
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			total := op.Read() + U(arriving.Read())

			if !yield(op.Carrier(total)) {
				return
			}
		}
	}
}
