package hawkes

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
KernelInput is the decay rate and the age it is applied to.
*/
type KernelInput struct {
	Beta float64
	Age  float64
}

/*
Kernel owns exp(-beta*age). No event, side, storage or observation-window
policy belongs here.
*/
type Kernel struct {
	core.Base[KernelInput, float64]
}

func NewKernel() *Kernel {
	return &Kernel{}
}

func (op *Kernel) Next(
	in iter.Seq[core.Primitive[KernelInput, KernelInput]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for arriving := range in {
			input := arriving.Read()

			if !yield(op.Carrier(math.Exp(-input.Beta * input.Age))) {
				return
			}
		}
	}
}

func kernel(beta, age float64) float64 {
	return math.Exp(-beta * age)
}
