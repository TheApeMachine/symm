package temporal

import (
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
EnergyRateInput is one interval's value and its open-left time bounds.
*/
type EnergyRateInput struct {
	Value float64
	From  int64
	To    int64
}

/*
EnergyRates owns r² / elapsed seconds over interval arrivals.
*/
type EnergyRates struct {
	err error
	out float64
}

func NewEnergyRates() core.Primitive {
	return &EnergyRates{}
}

func (op *EnergyRates) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*EnergyRateInput)(arriving)
			elapsed := float64(input.To-input.From) / float64(time.Second)
			op.out = (input.Value * input.Value) / elapsed

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *EnergyRates) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
