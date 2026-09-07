package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"math"
)

// Log owns only its numeric operation. The configured left source remains
// connected; neither a yielded result nor an exhausted run can replace it.
type Log struct {
	core.PrimitiveError
	left    core.Primitive
	current core.Primitive
}

func NewLog(left core.Primitive) *Log { return &Log{left: left} }
func (operation *Log) Next(in core.Primitive) core.Primitive {
	result := core.Yield(operation.left, in, func(held, value float64) float64 { return math.Log(value) }, operation)
	if result != nil {
		operation.current = result
	}
	return result
}
func (operation *Log) Read() any { return core.To[any](operation.current) }
