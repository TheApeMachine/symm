package statistic

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Moments owns Welford's sufficient statistics in fixed numeric fields.
*/
type Moments struct {
	Count float64
	Mean  float64
	M2    float64
}

/*
MomentReading fixes the before/after facts of one observation.
*/
type MomentReading struct {
	Moments
	Prior           Moments
	Value           float64
	Delta           float64
	Variance        float64
	Dispersion      float64
	VarianceDefined bool
}

/*
Update applies Welford's recurrence, preserving the prior for causal scoring.
*/
func (moments *Moments) Update(value float64) MomentReading {
	prior := *moments
	moments.Count++
	delta := value - moments.Mean
	moments.Mean += delta / moments.Count
	moments.M2 += delta * (value - moments.Mean)
	reading := MomentReading{Moments: *moments, Prior: prior, Value: value, Delta: delta}
	reading.Summarize(*moments)
	return reading
}

/*
Shed preserves Bessel-corrected dispersion when reducing effective support.
*/
func (moments *Moments) Shed(retain float64) {
	if retain <= 0 || retain >= 1 || moments.Count <= 2 {
		return
	}

	moments.Retain(retain)
}

/*
Retain applies the requested mass ratio with the two-sample variance floor.
*/
func (moments *Moments) Retain(retain float64) {
	count := math.Max(2, moments.Count*retain)
	moments.M2 *= (count - 1) / (moments.Count - 1)
	moments.Count = count
}

/*
Summarize refreshes the post-policy sample moments without rewriting the prior.
*/
func (momentReading *MomentReading) Summarize(moments Moments) {
	momentReading.Moments = moments
	momentReading.VarianceDefined = moments.Count > 1
	momentReading.Variance = moments.M2 / (moments.Count - 1)
	momentReading.Dispersion = math.Sqrt(momentReading.Variance)
}

/*
Estimator owns online Welford moment accumulation as a Primitive.
*/
type Estimator struct {
	*core.PrimitiveError

	moments Moments
	reading MomentReading
}

func NewEstimator() *Estimator {
	return &Estimator{PrimitiveError: core.NewPrimitiveError()}
}

func (estimator *Estimator) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			val := *(*float64)(arriving)
			estimator.reading = estimator.moments.Update(val)

			if !yield(unsafe.Pointer(&estimator.reading)) {
				return
			}
		}
	}
}
