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
	stages []core.Primitive
}

/*
NewNumber instantiates a nomagique.Number composer with the given stages.
*/
func NewNumber(stages ...core.Primitive) *Number {
	return &Number{
		PrimitiveError: core.NewPrimitiveError(),
		stages:         stages,
	}
}

func (number *Number) Next(input iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	curr := input

	for _, stage := range number.stages {
		if stage == nil {
			continue
		}

		curr = stage.Next(curr)

		if curr == nil {
			break
		}
	}

	if curr == nil {
		return func(yield func(unsafe.Pointer) bool) {}
	}

	return func(yield func(unsafe.Pointer) bool) {
		defer func() {
			for _, stage := range number.stages {
				if stage != nil {
					if err := stage.Error(); err != nil {
						number.Error(err)
					}
				}
			}
		}()

		for output := range curr {
			if !yield(output) {
				return
			}
		}
	}
}
