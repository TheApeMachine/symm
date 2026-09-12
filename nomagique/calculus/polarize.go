package calculus

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
PolarizeInput is a signed value and the scale it is normalized against.
*/
type PolarizeInput struct {
	Value float64
	Scale float64
}

/*
PolarizeResult splits a signed value into nonnegative components.
*/
type PolarizeResult struct {
	Alpha           float64
	Beta            float64
	AlphaNormalized float64
	BetaNormalized  float64
	Scale           float64
	Value           float64
}

/*
Polarize splits a signed value into nonnegative components and normalizes
against a configured scale.
*/
type Polarize struct {
	err error
	out PolarizeResult
}

func NewPolarize() core.Primitive {
	return &Polarize{}
}

func (op *Polarize) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*PolarizeInput)(arriving)
			alpha := input.Value
			beta := -input.Value

			if alpha < 0 {
				alpha = 0
			}

			if beta < 0 {
				beta = 0
			}

			op.out = PolarizeResult{
				Alpha: alpha,
				Beta:  beta,
				Scale: input.Scale,
			}

			if input.Scale > 0 {
				op.out.AlphaNormalized = alpha / (alpha + input.Scale)
				op.out.BetaNormalized = beta / (beta + input.Scale)
			}

			op.out.Value = op.out.AlphaNormalized - op.out.BetaNormalized

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Polarize) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
