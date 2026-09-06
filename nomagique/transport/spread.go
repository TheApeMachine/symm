package transport

import "github.com/theapemachine/symm/nomagique/core"

// Spread presents collection members as individual yields. Collection shape
// conversion belongs here, not in arithmetic or an estimator.
type Spread[T any] struct {
	core.PrimitiveError
	output  core.Primitive
	current core.Primitive
}

func NewSpread[T any]() *Spread[T] { return &Spread[T]{} }
func (spread *Spread[T]) Next(in core.Primitive) core.Primitive {
	if spread.output == nil {
		values := []core.Primitive{}
		core.Yield(
			NewIO(core.From(0)),
			in,
			func(held int, batch []T) int {
				for _, value := range batch {
					values = append(values, core.From(value))
				}
				return held
			},
			spread,
		)
		spread.output = NewIO(values...)
	}
	value := spread.output.Next(nil)
	if value == nil {
		spread.output = nil
	} else {
		spread.current = value
	}
	return value
}
func (spread *Spread[T]) Read() any { return core.To[any](spread.current) }
