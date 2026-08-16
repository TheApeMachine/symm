package calculus

import "github.com/theapemachine/symm/nomagique/types"

/*
Difference subtracts "right" from "left" and writes "result".
*/
type Difference struct {
	initial types.Input[scalarMap]
	next    types.Input[scalarMap]
}

var _ types.IO[scalarMap] = (*Difference)(nil)

func NewDifference(initial types.Input[scalarMap]) *Difference {
	return &Difference{initial: initial, next: types.NewInput[scalarMap]()}
}

func (difference *Difference) Write(input types.IO[scalarMap]) {
	mapping, err := stageScalar(input, "difference")
	if err != nil {
		difference.next = types.NewErrorInput(mapping, err)
		return
	}
	difference.next = scalarInput(mapping)
}

func (difference *Difference) Read() types.IO[scalarMap] {
	if difference.next.Error() != "" {
		return difference.next
	}

	mapping := difference.next.Project().Read()
	left, hasLeft := scalar(mapping, "left")
	right, hasRight := scalar(mapping, "right")
	if !hasLeft || !hasRight {
		difference.next = types.NewErrorInput(mapping,
			scalarValidation("difference", "missing left or right"))
		return difference.next
	}
	if !finite(left, right) {
		difference.next = types.NewErrorInput(mapping,
			scalarValidation("difference", "operands must be finite"))
		return difference.next
	}

	putScalar(mapping, "result", left-right)
	difference.initial = scalarInput(mapping)
	difference.next = scalarInput(mapping)
	return difference.next
}

func (difference *Difference) Project() types.Value[scalarMap] { return difference.next.Project() }
func (difference *Difference) Error() string                   { return difference.next.Error() }
func (difference *Difference) Close() error {
	if difference.initial != nil {
		if err := difference.initial.Close(); err != nil {
			return err
		}
	}
	if difference.next != nil {
		if err := difference.next.Close(); err != nil {
			return err
		}
	}
	difference.next = types.NewInput[scalarMap]()
	return nil
}
