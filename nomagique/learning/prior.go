package learning

import (
	"math"

	"github.com/theapemachine/errnie"
)

/*
Prior summarizes completed outcomes with weighted, online Welford moments.
Weight is observation authority, not reward magnitude: a large outcome cannot
give itself extra influence. No individual historical outcomes are retained.
*/
type Prior struct {
	memory        float64
	samples       uint64
	pending       uint64
	mean          float64
	weight        float64
	squaredWeight float64
	deviation     float64
	lastEpoch     uint64
}

/*
NewPrior constructs an online Welford prior with an optional exponential
retention memory window. When memory is greater than 1, older evidence decays
with every observation. When memory is not provided or <= 1, cumulative
un-decayed Welford is preserved.
*/
func NewPrior(memory ...float64) *Prior {
	prior := &Prior{}

	if len(memory) > 0 && memory[0] > 1 {
		prior.memory = memory[0]
	}

	return prior
}

func (prior *Prior) age(currentEpoch uint64) {
	if prior.memory <= 1 || prior.weight == 0 || currentEpoch <= prior.lastEpoch {
		if currentEpoch > prior.lastEpoch {
			prior.lastEpoch = currentEpoch
		}

		return
	}

	gap := float64(currentEpoch - prior.lastEpoch)
	decay := math.Pow(1.0-1.0/prior.memory, gap)
	prior.weight *= decay
	prior.squaredWeight *= decay * decay
	prior.deviation *= decay
	prior.lastEpoch = currentEpoch
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
	// Depth is how many context tokens this reading was conditioned on. Zero
	// is the key's unconditioned evidence; the caller's full context length is
	// the most specific answer available.
	Depth             int
	Samples           uint64
	Defined           bool
	Mean              float64
	Variance          float64
	VarianceDefined   bool
	Support           float64
	Maturity          float64
	EvidenceAuthority float64 // Retained authority-weighted input authority, independent of reward.
	Authority         float64
	Memory            float64
}

/*
Observe incorporates one completed outcome with authority in [0, 1], derived
at issue time. Zero authority records completion without
inventing trusted evidence. A measured zero with positive authority is evidence.
When an epoch is supplied, decay accounts for the resolution gap since this prior's
last update, ensuring dormant leaves age with the model.
*/
func (prior *Prior) Observe(value, authority float64, epoch ...uint64) error {
	if authority < 0 || authority > 1 {
		return errnie.Err(errnie.Validation, "prior: authority must be in [0, 1]", nil)
	}

	prior.samples++

	if authority == 0 {
		return nil
	}

	if len(epoch) > 0 {
		prior.age(epoch[0])
	}

	if len(epoch) == 0 && prior.memory > 1 {
		decay := 1.0 - 1.0/prior.memory
		prior.weight *= decay
		prior.squaredWeight *= decay * decay
		prior.deviation *= decay
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

/* Reading ages retained evidence to the supplied epoch and returns its estimate. */
func (prior *Prior) Reading(epoch ...uint64) PriorReading {
	if len(epoch) > 0 {
		prior.age(epoch[0])
	}

	reading := PriorReading{Samples: prior.samples, Memory: prior.memory}

	if prior.weight == 0 {
		return reading
	}

	reading.Defined = true
	reading.Mean = prior.mean
	reading.EvidenceAuthority = prior.squaredWeight / prior.weight
	reading.Support = prior.weight * prior.weight / prior.squaredWeight

	if reading.Support <= 1 {
		return reading
	}

	degrees := prior.weight - prior.squaredWeight/prior.weight

	if degrees <= 0 {
		return reading
	}

	reading.VarianceDefined = true
	// Reliability-weighted sample variance: M2 / (sum(w) - sum(w*w)/sum(w)).
	// https://numpy.org/doc/stable/reference/generated/numpy.cov.html#notes
	reading.Variance = prior.deviation / degrees

	if reading.Variance < 0 {
		reading.Variance = 0
	}

	reading.Maturity = (reading.Support - 1) / reading.Support
	power := reading.Mean * reading.Mean
	totalPower := power + reading.Variance

	if totalPower > 0 {
		inputAuthority := reading.EvidenceAuthority
		reading.Authority = reading.Maturity * inputAuthority * power / totalPower
	}

	return reading
}
