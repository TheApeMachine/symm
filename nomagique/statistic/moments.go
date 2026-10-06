package statistic

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Estimator owns online Welford moment accumulation as a Primitive.

Each arriving *float64 is folded into the running moments, and the
before/after facts of that observation are yielded as one *[10]float64:

	[0] count       [1] mean        [2] m2
	[3] prior count [4] prior mean  [5] prior m2
	[6] value       [7] delta       [8] variance  [9] dispersion

Variance is m2 / (count - 1); it is only defined when count > 1.
*/
type Estimator struct {
	*core.PrimitiveError
	count float64
	mean  float64
	m2    float64
	out   [10]float64
}

func NewEstimator() *Estimator {
	return &Estimator{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Estimator) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			value := *(*float64)(arriving)
			op.out[3] = op.count
			op.out[4] = op.mean
			op.out[5] = op.m2

			op.count++
			delta := value - op.mean
			op.mean += delta / op.count
			op.m2 += delta * (value - op.mean)

			op.out[0] = op.count
			op.out[1] = op.mean
			op.out[2] = op.m2
			op.out[6] = value
			op.out[7] = delta
			op.out[8] = op.m2 / (op.count - 1)
			op.out[9] = math.Sqrt(op.out[8])

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Shed reduces the effective support of one Estimator while preserving its
Bessel-corrected dispersion. Each arriving *float64 is a retain ratio; ratios
outside (0, 1), or an estimator holding two or fewer observations, leave the
moments untouched. Count never drops below the two-sample variance floor.

It yields the estimator's current moments as *[3]float64 {count, mean, m2}.
*/
type Shed struct {
	*core.PrimitiveError
	estimator *Estimator
	out       [3]float64
}

func NewShed(estimator *Estimator) *Shed {
	return &Shed{
		PrimitiveError: core.NewPrimitiveError(),
		estimator:      estimator,
	}
}

func (op *Shed) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil || op.estimator == nil {
				op.Error(core.ErrShape)
				return
			}

			retain := *(*float64)(arriving)
			moments := op.estimator

			if retain > 0 && retain < 1 && moments.count > 2 {
				count := math.Max(2, moments.count*retain)
				moments.m2 *= (count - 1) / (moments.count - 1)
				moments.count = count
			}

			op.out[0] = moments.count
			op.out[1] = moments.mean
			op.out[2] = moments.m2

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
