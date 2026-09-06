package transport

import "github.com/theapemachine/symm/nomagique/core"

// Apply binds an argument stream to a target. It forwards every target yield,
// including its run boundary, without inspecting either endpoint.
type Apply struct {
	core.PrimitiveError
	target core.Primitive
	input  core.Primitive
}

func NewApply(target, input core.Primitive) *Apply { return &Apply{target: target, input: input} }
func (call *Apply) Next(core.Primitive) core.Primitive {
	value := call.target.Next(call.input)
	call.Error(call.target.Error())
	return value
}
func (call *Apply) Read() any { return call.target.Read() }
