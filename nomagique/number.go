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
	return func(yield func(unsafe.Pointer) bool) {
		if number.Error() != nil {
			return
		}

		curr := input

		if curr == nil {
			curr = func(yield func(unsafe.Pointer) bool) {}
		}

		for _, stage := range number.Stages {
			if stage == nil {
				number.Error(core.ErrShape)
				return
			}

			if err := stage.Error(); err != nil {
				number.Error(err)
				return
			}

			curr = stage.Next(curr)

			if curr == nil {
				number.Error(core.ErrShape)
				return
			}
		}

		for ptr := range curr {
			if !yield(ptr) {
				return
			}
		}

		for _, stage := range number.Stages {
			number.Error(stage.Error())
		}
	}
}
