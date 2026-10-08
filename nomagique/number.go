package nomagique

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Number is the pipeline composer Primitive.
It is specifically named this way to make the consumer always
restate the core pillar of this package:

nomagique.Number
no, magic, number
*/
type Number struct {
	*core.PrimitiveError
	Stages []core.Primitive
}

/*
NewNumber instantiates a nomagique.Number composer with the given stages.
*/
func NewNumber(stages ...core.Primitive) *Number {
	return &Number{
		PrimitiveError: core.NewPrimitiveError(),
		Stages:         stages,
	}
}

func (number *Number) Next(input iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	curr := input

	if curr == nil {
		curr = func(yield func(unsafe.Pointer) bool) {}
	}

	for _, stage := range number.Stages {
		if curr == nil {
			break
		}

		curr = stage.Next(curr)
	}

	if curr == nil {
		return func(yield func(unsafe.Pointer) bool) {}
	}

	return curr
}
