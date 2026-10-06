package statistic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
WeightedMean owns sum(w x) / sum(w). Each arrival is *[2]float64
{weight, value}.
*/
type WeightedMean struct {
	*core.PrimitiveError
	mass  float64
	total float64
	out   float64
}

func NewWeightedMean() *WeightedMean {
	return &WeightedMean{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *WeightedMean) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			item := (*[2]float64)(arriving)
			op.mass += item[0]
			op.total += item[0] * item[1]

			op.out = 0

			if op.mass != 0 {
				op.out = op.total / op.mass
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
WeightedVariance owns E_w[x²] - E_w[x]². Each arrival is *[2]float64
{weight, value}.
*/
type WeightedVariance struct {
	*core.PrimitiveError
	mass   float64
	first  float64
	second float64
	out    float64
}

func NewWeightedVariance() *WeightedVariance {
	return &WeightedVariance{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *WeightedVariance) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			item := (*[2]float64)(arriving)
			op.mass += item[0]
			op.first += item[0] * item[1]
			op.second += item[0] * item[1] * item[1]

			op.out = 0

			if op.mass != 0 {
				mean := op.first / op.mass
				val := op.second/op.mass - mean*mean

				if val < 0 {
					val = 0
				}

				op.out = val
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
