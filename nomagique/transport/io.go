package transport

import "github.com/theapemachine/symm/nomagique/core"

// IO presents configured Primitive endpoints in order, then ends the run.
// Values are never decoded or advanced. Each IO owns its cursor; callers that
// need independent cursors construct independent IOs over the same objects.
type IO struct {
	core.PrimitiveError
	values []core.Primitive
	index  int
}

func NewIO(values ...core.Primitive) *IO {
	return &IO{values: append([]core.Primitive(nil), values...)}
}

func (stream *IO) Next(core.Primitive) core.Primitive {
	if stream.index == len(stream.values) {
		stream.index = 0
		return nil
	}

	value := stream.values[stream.index]
	stream.index++

	if value != nil {
		stream.Error(value.Error())
	}

	return value
}

func (stream *IO) Read() any {
	return append([]core.Primitive(nil), stream.values...)
}

/*
Reset rewinds the stream over a new set of endpoints, releasing the previous
run's references while retaining buffer capacity.

An owner that delivers many runs would otherwise construct one stream per run,
which is where the graph spends most of its allocation. Reuse is the owner's
call: it must have drained the previous run, since the cursor and the values
both move here.
*/
func (stream *IO) Reset(values ...core.Primitive) {
	stream.reset()
	stream.values = append(stream.values, values...)
}

// reset releases the previous run's references while retaining buffer capacity.
// Errors have already been collected by the owner before this run is reused.
func (stream *IO) reset() {
	clear(stream.values)
	stream.values = stream.values[:0]
	stream.index = 0
	stream.PrimitiveError = core.PrimitiveError{}
}

// appendRun captures opaque deliveries, including errors reported on final nil.
// Failures belong to the caller, not subsequent consumers of the captured values.
// The owner must drain the run before lending this buffer to another operation.
func (stream *IO) appendRun(operation, input core.Primitive) error {
	var failures core.PrimitiveError
	failures.Error(operation.Error())

	for value := operation.Next(input); value != nil; value = operation.Next(input) {
		failures.Error(value.Error())
		stream.values = append(stream.values, value)
	}

	return failures.Error(operation.Error())
}
