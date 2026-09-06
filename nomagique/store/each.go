package store

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Each routes every value of a run through an operation, one at a time.

Without it the algebra can accumulate a run but cannot transform one: an
operation shown a run folds it into its register, so a square over four
values is the square of the last of them rather than four squares. Each is
the arity-preserving pass: what arrives as four values leaves as four values,
each of them stepped through the operation on its own.

That is what dissolves a whole class of apparent Primitives. Counting is
summing after replacing every member with one, and an operation that ignores
what it is shown and answers with what it holds is a carrier, so a count is
an Add over an Each of a carrier and nothing more. A mean is one such sum
divided by another.

It holds no state of its own. The run belongs to the source, and the pass
ends when the source says it has nothing more.
*/
type Each struct {
	core.PrimitiveError
	source    core.Primitive
	operation core.Primitive
}

/*
NewEach configures the run being routed and the operation every value of it
is stepped through.
*/
func NewEach(source, operation core.Primitive) *Each {
	return &Each{
		source:    source,
		operation: operation,
	}
}

/*
Next hands over the operation's answer for the next value of the run, and nil
once the source has ended it. The caller is passed through untouched, so the
source keeps the run for whoever asked rather than for this.
*/
func (each *Each) Next(in core.Primitive) core.Primitive {
	value := each.source.Next(in)

	if value == nil {
		return nil
	}

	return each.operation.Next(value)
}

/*
Read surfaces the operation's current answer for the boundary.
*/
func (each *Each) Read() any {
	return each.operation.Read()
}
