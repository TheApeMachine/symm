package equation

import (
	"iter"
	"time"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
EnergyRateInput is one interval's value and its open-left time bounds.
*/
type EnergyRateInput[U core.Floating] struct {
	Value U
	From  int64
	To    int64
}

/*
EnergyRates owns r² / elapsed seconds over interval arrivals.
*/
type EnergyRates[U core.Floating] struct {
	core.Base[EnergyRateInput[U], U]
}

func NewEnergyRates[U core.Floating]() *EnergyRates[U] {
	return &EnergyRates[U]{}
}

func (op *EnergyRates[U]) Next(
	in iter.Seq[core.Primitive[EnergyRateInput[U], EnergyRateInput[U]]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			input := arriving.Read()
			elapsed := U(float64(input.To-input.From) / float64(time.Second))

			if !yield(op.Carrier((input.Value * input.Value) / elapsed)) {
				return
			}
		}
	}
}
