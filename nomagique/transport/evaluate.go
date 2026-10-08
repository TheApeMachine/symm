package transport

import (
	"errors"
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Evaluate is the Go boundary for a graph that returns exactly one observation.
Stateful graphs belong to one caller; Evaluate adds neither a scheduler nor a
second execution protocol. Next drives the held operation and yields each
output; the operation's own error state and any violation of the single-output
expectation are recorded in Error().
*/
type Evaluate struct {
	err       error
	operation core.Primitive
}

/*
NewEvaluate instantiates an Evaluate Primitive around one operation.
*/
func NewEvaluate(operation core.Primitive) core.Primitive {
	return &Evaluate{operation: operation}
}

func (op *Evaluate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if op.operation == nil {
			op.Error(fmt.Errorf("%w: evaluation requires an operation", core.ErrShape))
			return
		}

		count := 0

		for out := range op.operation.Next(in) {
			count++

			if !yield(out) {
				return
			}
		}

		if err := op.operation.Error(); err != nil {
			op.Error(err)
		}

		if count != 1 {
			op.Error(fmt.Errorf(
				"%w: primitive evaluation: expected one output, received %d",
				core.ErrShape,
				count,
			))
		}
	}
}

func (op *Evaluate) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
