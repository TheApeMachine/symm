package statistic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Weighted is one observation with its weight as a Primitive.
When stepped without input, it yields its weight then its value as scalar pointers.
*/
type Weighted struct {
	*core.PrimitiveError
	Weight float64
	Value  float64
}

func NewWeighted(weight, value float64) *Weighted {
	return &Weighted{
		PrimitiveError: core.NewPrimitiveError(),
		Weight:         weight,
		Value:          value,
	}
}

func (weighted *Weighted) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if !yield(unsafe.Pointer(&weighted.Weight)) {
			return
		}

		yield(unsafe.Pointer(&weighted.Value))
	}
}

/*
WeightedMean owns sum(w x) / sum(w).
*/
type WeightedMean struct {
	*core.PrimitiveError

	mass  float64
	total float64
	out   float64
}

func NewWeightedMean() *WeightedMean {
	return &WeightedMean{PrimitiveError: core.NewPrimitiveError()}
}

func (weightedMean *WeightedMean) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			item := *(*Weighted)(arriving)
			weightedMean.mass += item.Weight
			weightedMean.total += item.Weight * item.Value

			if weightedMean.mass != 0 {
				weightedMean.out = weightedMean.total / weightedMean.mass
			} else {
				weightedMean.out = 0
			}

			if !yield(unsafe.Pointer(&weightedMean.out)) {
				return
			}
		}
	}
}

/*
WeightedVariance owns E_w[x²] - E_w[x]².
*/
type WeightedVariance struct {
	*core.PrimitiveError

	mass   float64
	first  float64
	second float64
	out    float64
}

func NewWeightedVariance() *WeightedVariance {
	return &WeightedVariance{PrimitiveError: core.NewPrimitiveError()}
}

func (weightedVariance *WeightedVariance) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			item := *(*Weighted)(arriving)
			weightedVariance.mass += item.Weight
			weightedVariance.first += item.Weight * item.Value
			weightedVariance.second += item.Weight * item.Value * item.Value

			if weightedVariance.mass != 0 {
				mean := weightedVariance.first / weightedVariance.mass
				val := weightedVariance.second/weightedVariance.mass - mean*mean

				if val < 0 {
					val = 0
				}

				weightedVariance.out = val
			} else {
				weightedVariance.out = 0
			}

			if !yield(unsafe.Pointer(&weightedVariance.out)) {
				return
			}
		}
	}
}
