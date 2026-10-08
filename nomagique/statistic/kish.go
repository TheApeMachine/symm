package statistic

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Kish owns (sum w)² / sum(w²), the effective sample size of a weight stream.
*/
type Kish struct {
	err    error
	sum    float64
	energy float64
	out    float64
}

func NewKish() core.Primitive {
	return &Kish{}
}

func (op *Kish) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			val := *(*float64)(arriving)
			op.sum += val
			op.energy += val * val

			if op.energy == 0 {
				op.out = 0
			} else {
				op.out = (op.sum * op.sum) / op.energy
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Kish) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
KishMaturity maps effective sample support to the normative maturity measure:

	Maturity = 0             when N_eff <= 1
	Maturity = 1 - 1/N_eff   otherwise
*/
type KishMaturity struct {
	err error
	out float64
}

func NewKishMaturity() core.Primitive {
	return &KishMaturity{}
}

func (op *KishMaturity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			effective := *(*float64)(arriving)

			if effective <= 1 {
				op.out = 0
			} else {
				op.out = 1 - 1/effective
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *KishMaturity) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
