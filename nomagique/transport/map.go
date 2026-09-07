package transport

import "github.com/theapemachine/symm/nomagique/core"

// Map applies one configured Primitive to each incoming value. Each application
// is drained fully; a mapper may yield zero, one, or many values.
type Map struct {
	core.PrimitiveError
	operation core.Primitive
	output    core.Primitive
	current   core.Primitive
	buffer    IO
	input     IO
	member    [1]core.Primitive
}

func NewMap(operation core.Primitive) *Map { return &Map{operation: operation} }
func (mapping *Map) Next(in core.Primitive) core.Primitive {
	if mapping.output == nil {
		if in != nil {
			mapping.Error(in.Error())

			for value := in.Next(nil); value != nil; value = in.Next(nil) {
				mapping.Error(value.Error())
				mapping.member[0] = value
				mapping.input = IO{values: mapping.member[:]}
				mapping.Error(mapping.buffer.appendRun(mapping.operation, &mapping.input))
			}

			mapping.Error(in.Error())
		}

		mapping.output = &mapping.buffer
	}
	value := mapping.output.Next(nil)

	if value == nil {
		mapping.output = nil
		mapping.buffer.reset()
		mapping.member[0] = nil
		mapping.input = IO{}

		return nil
	}

	mapping.current = value
	return value
}
func (mapping *Map) Read() any { return core.To[any](mapping.current) }
