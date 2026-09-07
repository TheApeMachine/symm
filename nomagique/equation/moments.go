package equation

import (
	"github.com/theapemachine/symm/nomagique/core"
	"math"
)

/* Moments owns Welford's sufficient statistics in fixed numeric fields. */
type Moments struct{ Count, Mean, M2 float64 }

/* MomentReading fixes the before/after facts of one observation. */
type MomentReading struct {
	Moments
	Prior                Moments
	Value, Delta         float64
	Variance, Dispersion float64
	VarianceDefined      bool
}

/* Update applies Welford's recurrence, preserving the prior for causal scoring. */
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

/* Shed preserves Bessel-corrected dispersion when reducing effective support. */
func (moments *Moments) Shed(retain float64) {
	if retain <= 0 || retain >= 1 || moments.Count <= 2 {
		return
	}
	moments.Retain(retain)
}

/* Retain applies the requested mass ratio with the two-sample variance floor. */
func (moments *Moments) Retain(retain float64) {
	count := math.Max(2, moments.Count*retain)
	moments.M2 *= (count - 1) / (moments.Count - 1)
	moments.Count = count
}

/* Summarize refreshes the post-policy sample moments without rewriting the prior. */
func (reading *MomentReading) Summarize(moments Moments) {
	reading.Moments = moments
	reading.VarianceDefined = moments.Count > 1
	reading.Variance = moments.M2 / (moments.Count - 1)
	reading.Dispersion = math.Sqrt(reading.Variance)
}

/* Fields materializes names only at the generic record boundary. */
func (reading MomentReading) Fields() map[string]core.Primitive {
	return core.To[map[string]core.Primitive](core.Record(map[string]any{
		"count": reading.Count, "mean": reading.Mean, "m2": reading.M2,
		"prior_count": reading.Prior.Count, "prior_mean": reading.Prior.Mean, "prior_m2": reading.Prior.M2,
		"value": reading.Value, "delta": reading.Delta,
		"variance": reading.Variance, "dispersion": reading.Dispersion, "variance_defined": reading.VarianceDefined,
	}))
}
