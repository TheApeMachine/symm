package transport

import (
	"errors"
	"fmt"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Evaluate is the Go boundary for a graph that returns exactly one observation.
It presents an inert carrier through IO and drains the complete delivery run,
including the final nil. Reading only the first result would leave a graph in
its previous run and lose the next event. Stateful graphs belong to one caller;
Evaluate adds neither a scheduler nor a second execution protocol.
*/
func Evaluate[Value any](operation, input core.Primitive) (Value, error) {
	var result Value
	if operation == nil {
		return result, fmt.Errorf("primitive evaluation: operation is required")
	}
	stream := NewIO(input)
	count := 0
	var failure error
	for output := operation.Next(stream); output != nil; output = operation.Next(stream) {
		count++
		result = core.To[Value](output)
		failure = errors.Join(failure, output.Error())
	}
	failure = errors.Join(failure, stream.Error(), operation.Error())
	if count != 1 {
		failure = errors.Join(failure, fmt.Errorf("primitive evaluation: expected one output, received %d", count))
	}
	if failure != nil {
		var zero Value
		return zero, failure
	}
	return result, nil
}
