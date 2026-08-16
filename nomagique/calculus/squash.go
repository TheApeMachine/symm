package calculus

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Squash maps positive evidence through a positive empirical scale:
value / (scale + value). Invalid or non-positive evidence maps to zero.
*/
type Squash struct {
	initial types.Input[scalarMap]
	next    types.Input[scalarMap]
}

var _ types.IO[scalarMap] = (*Squash)(nil)

func NewSquash(initial types.Input[scalarMap]) *Squash {
	return &Squash{initial: initial, next: types.NewInput[scalarMap]()}
}

func (squash *Squash) Write(input types.IO[scalarMap]) {
	mapping, err := stageScalar(input, "squash")
	if err != nil {
		squash.next = types.NewErrorInput(mapping, err)
		return
	}
	squash.next = scalarInput(mapping)
}

func (squash *Squash) Read() types.IO[scalarMap] {
	if squash.next.Error() != "" {
		return squash.next
	}

	mapping := squash.next.Project().Read()
	value, hasValue := scalar(mapping, "value")
	scale, hasScale := scalar(mapping, "scale")
	if !hasValue || !hasScale {
		squash.next = types.NewErrorInput(mapping,
			scalarValidation("squash", "missing value or scale"))
		return squash.next
	}

	result := 0.0
	if value > 0 && scale > 0 && !math.IsNaN(value) && !math.IsInf(value, 0) &&
		!math.IsNaN(scale) && !math.IsInf(scale, 0) {
		result = value / (scale + value)
	}

	putScalar(mapping, "result", result)
	squash.initial = scalarInput(mapping)
	squash.next = scalarInput(mapping)
	return squash.next
}

func (squash *Squash) Project() types.Value[scalarMap] { return squash.next.Project() }
func (squash *Squash) Error() string                   { return squash.next.Error() }
func (squash *Squash) Close() error {
	if squash.initial != nil {
		if err := squash.initial.Close(); err != nil {
			return err
		}
	}
	if squash.next != nil {
		if err := squash.next.Close(); err != nil {
			return err
		}
	}
	squash.next = types.NewInput[scalarMap]()
	return nil
}
