package statistic

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Weighted is one observation with its weight.
*/
type Weighted struct {
	Weight float64
	Value  float64
}

/*
WeightedMean owns sum(w x) / sum(w).
*/
type WeightedMean struct {
	err   error
	mass  float64
	total float64
	out   float64
}

func NewWeightedMean() core.Primitive {
	return &WeightedMean{}
}

func (op *WeightedMean) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			item := *(*Weighted)(arriving)
			op.mass += item.Weight
			op.total += item.Weight * item.Value

			if op.mass != 0 {
				op.out = op.total / op.mass
			} else {
				op.out = 0
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *WeightedMean) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
WeightedVariance owns E_w[x²] - E_w[x]².
*/
type WeightedVariance struct {
	err    error
	mass   float64
	first  float64
	second float64
	out    float64
}

func NewWeightedVariance() core.Primitive {
	return &WeightedVariance{}
}

func (op *WeightedVariance) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			item := *(*Weighted)(arriving)
			op.mass += item.Weight
			op.first += item.Weight * item.Value
			op.second += item.Weight * item.Value * item.Value

			if op.mass != 0 {
				mean := op.first / op.mass
				val := op.second/op.mass - mean*mean

				if val < 0 {
					val = 0
				}

				op.out = val
			} else {
				op.out = 0
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *WeightedVariance) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
