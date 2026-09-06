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
func (stream *IO) Read() any { return append([]core.Primitive(nil), stream.values...) }
