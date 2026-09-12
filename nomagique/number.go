package nomagique

import (
	"errors"
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
	err    error
	stages []core.Primitive
}

/*
NewNumber instantiates a nomagique.Number composer with the given stages.
*/
func NewNumber(stages ...core.Primitive) core.Primitive {
	return &Number{
		stages: stages,
	}
}

func (number *Number) Next(input iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	curr := input
	if curr == nil {
		curr = func(yield func(unsafe.Pointer) bool) {}
	}

	for _, stage := range number.stages {
		curr = stage.Next(curr)
	}

	return curr
}

func (number *Number) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			number.err = errors.Join(number.err, err)
		}
	}

	for _, stage := range number.stages {
		if err := stage.Error(); err != nil {
			number.err = errors.Join(number.err, err)
		}
	}

	return number.err
}
