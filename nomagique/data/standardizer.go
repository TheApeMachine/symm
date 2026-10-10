package data

import (
	"math"
	"sync"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
standardizer owns one metric stream's causal location and scale: the Welford
moments of every observation the stream has already produced. An observation
is standardized against the moments as they stood before it arrived and only
then folded in, so a z-score never contains its own value or any later one.
*/
type standardizer struct {
	mu      sync.Mutex
	count   float64
	mean    float64
	m2      float64
	modulus core.LogModulus
}

/*
standardizerKey identifies one metric stream. The epoch is part of the
identity so a replay of one epoch never inherits moments from another.
*/
type standardizerKey struct {
	epoch  int64
	source string
	label  string
	metric string
}

/*
standardizers holds every stream's standardizer. Producers derive their output
from a fresh ingress frame on every event, so moments carried on frames die
with the frame; every producer finalizes through this package, which makes it
the one owner that sees each stream from its first observation. Signals run on
separate goroutines, so lookup goes through sync.Map and each stream locks its
own moments.
*/
var standardizers sync.Map

/*
standardizerFor returns the stream's standardizer, creating it on first use.
*/
func standardizerFor(key standardizerKey) *standardizer {
	if found, ok := standardizers.Load(key); ok {
		return found.(*standardizer)
	}

	found, _ := standardizers.LoadOrStore(key, &standardizer{})

	return found.(*standardizer)
}

/*
step returns the center and scale of the observations seen before value, then
incorporates value. The scale is the sample standard deviation of those prior
observations, and it stays zero, meaning undefined, while core.PriorScale
refuses it: fewer than core.MinimumPrior prior observations (the sigma's own
relative standard error, about 1/sqrt(2(n-1)), still above core.Tolerance), no
dispersion, or a scale negligible next to max(|value|, |center|), the same
relative rule Joint applies. A scale that small is rounding residue of values
at that magnitude (a flat stream whose mean drifted by an ulp, or decayed
near-zero remnants), and dividing by it reports float noise as a deviation of
up to 1e19 sigma. The rule is relative only: an absolute floor would refuse
genuine dispersion in streams whose unit makes every value small, such as
micro-cap prices.
*/
func (state *standardizer) observe(raw float64, space Scale) (
	value float64, center float64, scale float64, defined bool,
) {
	state.mu.Lock()
	defer state.mu.Unlock()

	switch space {
	case ScaleLog:
		if raw <= 0 {
			return 0, 0, 0, false
		}

		value = math.Log(raw)
	case ScaleLogModulus:
		if value, defined = state.modulus.Step(raw); !defined {
			return 0, 0, 0, false
		}
	default:
		value = raw
	}

	center, scale = state.stepLocked(value)

	return value, center, scale, true
}

func (state *standardizer) stepLocked(value float64) (center float64, scale float64) {
	center = state.mean
	scale, _ = core.PriorScale(state.count, state.m2, value, center)

	state.count++
	delta := value - state.mean
	state.mean += delta / state.count
	state.m2 += delta * (value - state.mean)

	return center, scale
}
