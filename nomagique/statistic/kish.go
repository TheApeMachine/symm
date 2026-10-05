package statistic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Kish owns (sum w)² / sum(w²), the effective sample size of a weight stream.
*/
type Kish struct {
	*core.PrimitiveError
	sum    float64
	energy float64
	out    float64
}

func NewKish() *Kish {
	return &Kish{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Kish) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			val := *(*float64)(arriving)
			op.sum += val
			op.energy += val * val

			op.out = 0

			if op.energy != 0 {
				op.out = (op.sum * op.sum) / op.energy
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
KishMaturity maps effective sample support to the normative maturity measure:

	Maturity = 0             when N_eff <= 1
	Maturity = 1 - 1/N_eff   otherwise
*/
type KishMaturity struct {
	*core.PrimitiveError
	out float64
}

func NewKishMaturity() *KishMaturity {
	return &KishMaturity{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *KishMaturity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			effective := *(*float64)(arriving)

			op.out = 0

			if effective > 1 {
				op.out = 1 - 1/effective
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
