package calculus

import "github.com/theapemachine/symm/nomagique/types"

/*
Sum adds two finite scalar operands carried as "left" and "right" and writes
"result". It is the generic addition primitive behind accumulation and jumps.
*/
type Sum struct {
	initial types.Input[scalarMap]
	next    types.Input[scalarMap]
}

var _ types.IO[scalarMap] = (*Sum)(nil)

func NewSum(initial types.Input[scalarMap]) *Sum {
	return &Sum{initial: initial, next: types.NewInput[scalarMap]()}
}

func (sum *Sum) Write(input types.IO[scalarMap]) {
	mapping, err := stageScalar(input, "sum")
	if err != nil {
		sum.next = types.NewErrorInput(mapping, err)
		return
	}

	sum.next = scalarInput(mapping)
}

func (sum *Sum) Read() types.IO[scalarMap] {
	if sum.next.Error() != "" {
		return sum.next
	}

	mapping := sum.next.Project().Read()
	left, hasLeft := scalar(mapping, "left")
	right, hasRight := scalar(mapping, "right")

	if !hasLeft || !hasRight {
		sum.next = types.NewErrorInput(mapping,
			scalarValidation("sum", "missing left or right"))
		return sum.next
	}

	if !finite(left, right) {
		sum.next = types.NewErrorInput(mapping,
			scalarValidation("sum", "operands must be finite"))
		return sum.next
	}

	putScalar(mapping, "result", left+right)
	sum.initial = scalarInput(mapping)
	sum.next = scalarInput(mapping)

	return sum.next
}

func (sum *Sum) Project() types.Value[scalarMap] { return sum.next.Project() }
func (sum *Sum) Error() string                   { return sum.next.Error() }

func (sum *Sum) Close() error {
	if sum.initial != nil {
		if err := sum.initial.Close(); err != nil {
			return err
		}
	}
	if sum.next != nil {
		if err := sum.next.Close(); err != nil {
			return err
		}
	}
	sum.next = types.NewInput[scalarMap]()
	return nil
}
