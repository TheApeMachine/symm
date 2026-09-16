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
	return &Kish{PrimitiveError: core.NewPrimitiveError()}
}

func (kish *Kish) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			val := *(*float64)(arriving)
			kish.sum += val
			kish.energy += val * val

			if kish.energy == 0 {
				kish.out = 0
			} else {
				kish.out = (kish.sum * kish.sum) / kish.energy
			}

			if !yield(unsafe.Pointer(&kish.out)) {
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
	return &KishMaturity{PrimitiveError: core.NewPrimitiveError()}
}

func (kishMaturity *KishMaturity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			effective := *(*float64)(arriving)

			if effective <= 1 {
				kishMaturity.out = 0
			} else {
				kishMaturity.out = 1 - 1/effective
			}

			if !yield(unsafe.Pointer(&kishMaturity.out)) {
				return
			}
		}
	}
}
