package correlation

import (
	"iter"
	"math"
	"slices"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Dependence owns contemporaneous path diagnostics around an opaque estimator.
Each arrival is *[2][][2]float64{leftPrices, rightPrices}; it yields
[13]float64{correlation, covariance, support, leftEnergy, rightEnergy, defined,
leftReturns, rightReturns, leftEnergyRate, rightEnergyRate, definedAgain,
sharedTime, overlapDensity}.
*/
type Dependence struct {
	*core.PrimitiveError
	estimator    core.Primitive
	leftReturns  core.Primitive
	rightReturns core.Primitive
	query        [3][]float64
	rates        []float64
	out          [13]float64
}

func NewDependence(estimator core.Primitive) core.Primitive {
	return &Dependence{
		PrimitiveError: core.NewPrimitiveError(),
		estimator:      estimator,
		leftReturns:    NewReturns(),
		rightReturns:   NewReturns(),
	}
}

func (op *Dependence) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			input := (*[2][][2]float64)(arriving)
			var left, right [2][]float64

			for pointer := range op.leftReturns.Next(data.NewValue(input[0]).Next(nil)) {
				left = *(*[2][]float64)(pointer)
			}

			if err := op.leftReturns.Error(); err != nil {
				op.Error(err)
				return
			}

			for pointer := range op.rightReturns.Next(data.NewValue(input[1]).Next(nil)) {
				right = *(*[2][]float64)(pointer)
			}

			if err := op.rightReturns.Error(); err != nil {
				op.Error(err)
				return
			}

			leftEnergy, rightEnergy := 0.0, 0.0

			if len(left[1]) > 0 {
				leftEnergy = left[1][0]
			}

			if len(right[1]) > 0 {
				rightEnergy = right[1][0]
			}

			op.query[0] = left[0]
			op.query[1] = right[0]
			op.query[2] = []float64{leftEnergy, rightEnergy, 0}

			var estimate [6]float64

			for pointer := range op.estimator.Next(data.NewValue(op.query).Next(nil)) {
				estimate = *(*[6]float64)(pointer)
			}

			if err := op.estimator.Error(); err != nil {
				op.Error(err)
				return
			}

			leftCount := float64(len(left[0]) / 3)
			rightCount := float64(len(right[0]) / 3)
			shared, density := 0.0, 0.0

			if len(left[0]) >= 3 && len(right[0]) >= 3 {
				leftFrom := left[0][1]
				leftThrough := left[0][len(left[0])-1]
				rightFrom := right[0][1]
				rightThrough := right[0][len(right[0])-1]
				start := math.Max(leftFrom, rightFrom)
				end := math.Min(leftThrough, rightThrough)

				if end > start {
					shared = (end - start) / float64(time.Second)
				}
			}

			if shared > 0 {
				density = estimate[2] / shared
			}

			leftRate := math.NaN()
			rightRate := math.NaN()

			if count := len(left[0]) / 3; count > 0 {
				if cap(op.rates) < count {
					op.rates = make([]float64, count)
				} else {
					op.rates = op.rates[:count]
				}

				for i := 0; i < count; i++ {
					value := left[0][i*3]
					from := left[0][i*3+1]
					to := left[0][i*3+2]
					op.rates[i] = (value * value) / ((to - from) / float64(time.Second))
				}

				slices.Sort(op.rates)
				leftRate = (op.rates[(count-1)/2] + op.rates[count/2]) * 0.5
			}

			if count := len(right[0]) / 3; count > 0 {
				if cap(op.rates) < count {
					op.rates = make([]float64, count)
				} else {
					op.rates = op.rates[:count]
				}

				for i := 0; i < count; i++ {
					value := right[0][i*3]
					from := right[0][i*3+1]
					to := right[0][i*3+2]
					op.rates[i] = (value * value) / ((to - from) / float64(time.Second))
				}

				slices.Sort(op.rates)
				rightRate = (op.rates[(count-1)/2] + op.rates[count/2]) * 0.5
			}

			op.out = [13]float64{
				estimate[0],
				estimate[1],
				estimate[2],
				estimate[3],
				estimate[4],
				estimate[5],
				leftCount,
				rightCount,
				leftRate,
				rightRate,
				estimate[5],
				shared,
				density,
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
