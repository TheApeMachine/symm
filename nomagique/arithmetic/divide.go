package arithmetic

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Divide owns one field operation. Configuration supplies the value a run starts
from; recurrence and delivery remain separate Primitives.

Divide is itself Primitive[U, U]: what it hands over is its own accumulated
value, of the same type every time. T is only the type of what arrives on the
input run, which may differ from U (a narrower reading widened into a wider
running total).
*/
type Divide[U core.Floating] struct {
	core.Base[U, U]
}

/*
NewDivide creates a new Divide primitive with the given initial value. It initializes
the internal state and prepares the primitive for processing incoming values.
*/
func NewDivide[U core.Floating](current U) *Divide[U] {
	op := &Divide[U]{}
	op.Carrier(current)
	return op
}

/*
Next folds the incoming run into the value it was configured with and hands
that value over after every arrival. What it hands over is itself.
*/
func (op *Divide[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			total := op.Read() / U(arriving.Read())

			if !yield(op.Carrier(total)) {
				return
			}
		}
	}
}
