package transport

import (
	"github.com/theapemachine/symm/nomagique/core"
)

// Enumerate attaches a run-relative index to each opaque value. Indexing is
// structural transport, not a domain count or a statistical support estimate.
type Enumerate struct {
	core.PrimitiveError
	output, current core.Primitive
}

func NewEnumerate() *Enumerate { return &Enumerate{} }
func (enumerate *Enumerate) Next(in core.Primitive) core.Primitive {
	if enumerate.output == nil {
		values := []core.Primitive{}
		index := 0
		core.Yield(
			NewIO(core.From(0)),
			in,
			func(n int, v core.Primitive) int {
				values = append(values, core.From([]core.Primitive{core.From(float64(index)), v}))
				index++
				return n
			},
			enumerate,
		)
		enumerate.output = NewIO(values...)
	}
	value := enumerate.output.Next(nil)
	if value == nil {
		enumerate.output = nil
	} else {
		enumerate.current = value
	}
	return value
}
func (enumerate *Enumerate) Read() any { return core.To[any](enumerate.current) }
