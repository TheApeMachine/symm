package calculus

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Inverse maps counter-evidence through an empirical scale. Measured absence is
complete quiet and therefore maps to one without requiring a positive scale.
*/
type Inverse struct {
	initial types.Input[scalarMap]
	next    types.Input[scalarMap]
}

var _ types.IO[scalarMap] = (*Inverse)(nil)

func NewInverse(initial types.Input[scalarMap]) *Inverse {
	return &Inverse{initial: initial, next: types.NewInput[scalarMap]()}
}

func (inverse *Inverse) Write(input types.IO[scalarMap]) {
	mapping, err := stageScalar(input, "inverse")
	if err != nil {
		inverse.next = types.NewErrorInput(mapping, err)
		return
	}
	inverse.next = scalarInput(mapping)
}

func (inverse *Inverse) Read() types.IO[scalarMap] {
	if inverse.next.Error() != "" {
		return inverse.next
	}

	mapping := inverse.next.Project().Read()
	value, hasValue := scalar(mapping, "value")
	scale, hasScale := scalar(mapping, "scale")
	if !hasValue || !hasScale {
		inverse.next = types.NewErrorInput(mapping,
			scalarValidation("inverse", "missing value or scale"))
		return inverse.next
	}

	result := 0.0
	switch {
	case math.IsNaN(value) || math.IsInf(value, 0):
		result = 0
	case value < 0:
		result = 0
	case value == 0:
		result = 1
	case scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0):
		result = 0
	default:
		result = scale / (scale + value)
	}

	putScalar(mapping, "result", result)
	inverse.initial = scalarInput(mapping)
	inverse.next = scalarInput(mapping)
	return inverse.next
}

func (inverse *Inverse) Project() types.Value[scalarMap] { return inverse.next.Project() }
func (inverse *Inverse) Error() string                   { return inverse.next.Error() }
func (inverse *Inverse) Close() error {
	if inverse.initial != nil {
		if err := inverse.initial.Close(); err != nil {
			return err
		}
	}
	if inverse.next != nil {
		if err := inverse.next.Close(); err != nil {
			return err
		}
	}
	inverse.next = types.NewInput[scalarMap]()
	return nil
}
