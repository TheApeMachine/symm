package transport

import "github.com/theapemachine/symm/nomagique/core"

// Map applies one configured Primitive to each incoming value. Each application
// is drained fully; a mapper may yield zero, one, or many values.
type Map struct {
	core.PrimitiveError
	operation core.Primitive
	output    core.Primitive
	current   core.Primitive
}

func NewMap(operation core.Primitive) *Map { return &Map{operation: operation} }
func (mapping *Map) Next(in core.Primitive) core.Primitive {
	if mapping.output == nil {
		values := []core.Primitive{}
		core.Yield(
			NewIO(core.From(0)),
			in,
			func(held int, value core.Primitive) int {
				core.Yield(
					NewIO(core.From(0)),
					NewApply(mapping.operation, NewIO(value)),
					func(held int, result core.Primitive) int {
						values = append(values, result)
						return held
					},
					mapping,
				)
				return held
			},
			mapping,
		)
		mapping.output = NewIO(values...)
	}
	value := mapping.output.Next(nil)
	if value == nil {
		mapping.output = nil
	} else {
		mapping.current = value
	}
	return value
}
func (mapping *Map) Read() any { return core.To[any](mapping.current) }
