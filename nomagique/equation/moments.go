package equation

import "math"

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
func (reading *MomentReading) Summarize(moments Moments) {
	reading.Moments = moments
	reading.VarianceDefined = moments.Count > 1

	if reading.VarianceDefined {
		reading.Variance = moments.M2 / (moments.Count - 1)
		reading.Dispersion = math.Sqrt(reading.Variance)
		return
	}

	reading.Variance = 0
	reading.Dispersion = 0
}
