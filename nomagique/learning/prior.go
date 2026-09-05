package learning

import "github.com/theapemachine/errnie"

/*
Prior summarizes completed outcomes with weighted, online Welford moments.
Weight is observation authority, not reward magnitude: a large outcome cannot
give itself extra influence. No individual historical outcomes are retained.
*/
type Prior struct {
	samples       uint64
	pending       uint64
	mean          float64
	weight        float64
	squaredWeight float64
	deviation     float64
}

/*
PriorReading distinguishes absent evidence and unestimable dispersion from
observed zeros. Support is Kish effective sample size, not a claim that input
experiences are statistically independent. Variance is the reliability-weighted
sample variance; Maturity uses the Measurement contract's 1 - 1/support.

Authority combines maturity, the authority-weighted mean input authority, and
mean-square signal power relative to observed outcome dispersion. Uniformly
weak inputs cannot acquire full authority just by accumulating. It measures
support for the mean's departure from zero;
it is not a probability of success or a substitute for Mean in accounting.
*/
type PriorReading struct {
	Samples         uint64
	Defined         bool
	Mean            float64
	Variance        float64
	VarianceDefined bool
	Support         float64
	Maturity        float64
	Authority       float64
}

/*
Observe incorporates one completed outcome with authority in [0, 1], derived
at issue time. Zero authority records completion without
inventing trusted evidence. A measured zero with positive authority is evidence.
*/
func (prior *Prior) Observe(value, authority float64) error {
	if authority < 0 || authority > 1 {
		return errnie.Err(errnie.Validation, "prior: authority must be in [0, 1]", nil)
	}

	prior.samples++

	if authority == 0 {
		return nil
	}

	if prior.weight == 0 {
		prior.mean = value
		prior.weight = authority
		prior.squaredWeight = authority * authority

		return nil
	}

	prior.weight += authority
	prior.squaredWeight += authority * authority
	difference := value - prior.mean
	prior.mean += authority / prior.weight * difference
	prior.deviation += authority * difference * (value - prior.mean)

	return nil
}

/* Reading returns the current estimate without modifying its evidence. */
func (prior *Prior) Reading() PriorReading {
	reading := PriorReading{Samples: prior.samples}

	if prior.weight == 0 {
		return reading
	}

	reading.Defined = true
	reading.Mean = prior.mean
	reading.Support = prior.weight * prior.weight / prior.squaredWeight

	if reading.Support <= 1 {
		return reading
	}

	degrees := prior.weight - prior.squaredWeight/prior.weight
	reading.VarianceDefined = true
	// Reliability-weighted sample variance: M2 / (sum(w) - sum(w*w)/sum(w)).
	// https://numpy.org/doc/stable/reference/generated/numpy.cov.html#notes
	reading.Variance = prior.deviation / degrees
	reading.Maturity = (reading.Support - 1) / reading.Support
	power := reading.Mean * reading.Mean
	totalPower := power + reading.Variance

	if totalPower > 0 {
		inputAuthority := prior.squaredWeight / prior.weight
		reading.Authority = reading.Maturity * inputAuthority * power / totalPower
	}

	return reading
}
