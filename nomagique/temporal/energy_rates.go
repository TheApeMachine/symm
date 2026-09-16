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
	*core.PrimitiveError

	out float64
}

func NewEnergyRates() *EnergyRates {
	return &EnergyRates{PrimitiveError: core.NewPrimitiveError()}
}

func (energyRates *EnergyRates) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*EnergyRateInput)(arriving)
			elapsed := float64(input.To-input.From) / float64(time.Second)
			energyRates.out = (input.Value * input.Value) / elapsed

			if !yield(unsafe.Pointer(&energyRates.out)) {
				return
			}
		}
	}
}
