package calculus

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Positive projects a finite scalar onto the non-negative half-line.
*/
type Positive struct {
	initial types.Input[scalarMap]
	next    types.Input[scalarMap]
}

var _ types.IO[scalarMap] = (*Positive)(nil)

func NewPositive(initial types.Input[scalarMap]) *Positive {
	return &Positive{initial: initial, next: types.NewInput[scalarMap]()}
}

func (positive *Positive) Write(input types.IO[scalarMap]) {
	mapping, err := stageScalar(input, "positive")
	if err != nil {
		positive.next = types.NewErrorInput(mapping, err)
		return
	}
	positive.next = scalarInput(mapping)
}

func (positive *Positive) Read() types.IO[scalarMap] {
	if positive.next.Error() != "" {
		return positive.next
	}

	mapping := positive.next.Project().Read()
	value, found := scalar(mapping, "value")
	if !found {
		positive.next = types.NewErrorInput(mapping,
			scalarValidation("positive", "missing value"))
		return positive.next
	}
	if !finite(value) {
		positive.next = types.NewErrorInput(mapping,
			scalarValidation("positive", "value must be finite"))
		return positive.next
	}

	putScalar(mapping, "result", math.Max(0, value))
	positive.initial = scalarInput(mapping)
	positive.next = scalarInput(mapping)
	return positive.next
}

func (positive *Positive) Project() types.Value[scalarMap] { return positive.next.Project() }
func (positive *Positive) Error() string                   { return positive.next.Error() }
func (positive *Positive) Close() error {
	if positive.initial != nil {
		if err := positive.initial.Close(); err != nil {
			return err
		}
	}
	if positive.next != nil {
		if err := positive.next.Close(); err != nil {
			return err
		}
	}
	positive.next = types.NewInput[scalarMap]()
	return nil
}
