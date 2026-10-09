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
	mu    sync.Mutex
	count float64
	mean  float64
	m2    float64
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
observations; it stays zero, meaning undefined, until at least two prior
observations differ.
*/
func (state *standardizer) step(value float64) (center float64, scale float64) {
	state.mu.Lock()
	defer state.mu.Unlock()

	center = state.mean

	if state.count > core.Unit {
		scale = math.Sqrt(state.m2 / (state.count - core.Unit))
	}

	state.count++
	delta := value - state.mean
	state.mean += delta / state.count
	state.m2 += delta * (value - state.mean)

	return center, scale
}
