package transport

import (
	"errors"
	"fmt"
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Evaluate is the Go boundary for a graph that returns exactly one observation.
Stateful graphs belong to one caller; Evaluate adds neither a scheduler nor a
second execution protocol.
*/
func Evaluate[T, U any](
	operation core.Primitive[T, U],
	in iter.Seq[core.Primitive[T, T]],
) (U, error) {
	var result U

	if operation == nil {
		return result, fmt.Errorf("primitive evaluation: operation is required")
	}

	count := 0

	for out := range operation.Next(in) {
		count++
		result = out.Read()
	}

	err := operation.Error()

	if count != 1 {
		err = errors.Join(err, fmt.Errorf("primitive evaluation: expected one output, received %d", count))
	}

	if err != nil {
		var zero U
		return zero, err
	}

	return result, nil
}
