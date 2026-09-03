package equation

import (
	"math"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
RenewalRate (formerly Acceleration) is an event-accumulated rate equation.
An adaptive median sizes each accumulation span. When accumulated volume crosses
the emergent threshold, it computes the throughput rate and log-signal change.

Fulfills Tier 4 Equation rules:
- Zero Wire blocks.
- Embeds lower-tier primitives by value.
- Owns its memory privately (no global symbol table).
- Zero allocations in steady state.
*/
type RenewalRate struct {
	// Composed lower-tier primitives
	spanStore store.Store

	// Private state owned directly on the struct
	accumulated types.Scalar
	lastSample  types.Scalar
	spanStart   float64
	hasSample   bool
	hasOrigin   bool

	// Latest evaluation outputs
	rate     types.Scalar
	change   types.Scalar
	closed   bool
	maturity types.Scalar
	spans    float64
}

// NewRenewalRate initializes the equation with an adaptive ADWIN window.
func NewRenewalRate() *RenewalRate {
	return &RenewalRate{
		spanStore: store.Store{
			Type:     store.DynamicRing,
			Adaptive: adaptive.Window{Type: adaptive.ADWIN},
			Reduce:   statistic.MedianReduction,
		},
	}
}

/*
Step evaluates one arrival increment and signal sample.
Returns the computed rate as the primary carrier signal.
*/
func (eq *RenewalRate) Step(increment, sample types.Scalar, timestamp float64) types.Scalar {
	if !eq.hasOrigin {
		eq.spanStart = timestamp
		eq.hasOrigin = true
	}

	// 1. Update the adaptive span target using the incoming increment
	targetSpan := eq.spanStore.Step(increment)

	if targetSpan <= 0 {
		targetSpan = increment
	}

	// 2. Accumulate increment
	eq.accumulated += increment
	elapsed := timestamp - eq.spanStart

	// 3. Evaluate closure threshold: Total >= Target && Elapsed > 0
	if eq.accumulated >= targetSpan && elapsed > 0 {
		// Calculate renewal rate: Total / Elapsed
		eq.rate = eq.accumulated / types.Scalar(elapsed)

		// Calculate logarithmic change of the primary signal: ln(x_t / x_{t-1})
		if eq.hasSample && eq.lastSample > 0 && sample > 0 {
			eq.change = types.Scalar(math.Log(float64(sample / eq.lastSample)))
		} else {
			eq.change = 0
		}

		// Reset span state
		eq.lastSample = sample
		eq.hasSample = true
		eq.spanStart = timestamp
		eq.accumulated = 0
		eq.closed = true
		eq.spans++

		// Empirical maturity over completed spans: n / (n + 1)
		eq.maturity = types.Scalar(eq.spans / (eq.spans + 1.0))
	} else {
		eq.closed = false
	}

	return eq.rate
}

// Zero-cost auxiliary property accessors:
func (eq *RenewalRate) Rate() types.Scalar     { return eq.rate }
func (eq *RenewalRate) Change() types.Scalar   { return eq.change }
func (eq *RenewalRate) Closed() bool           { return eq.closed }
func (eq *RenewalRate) Maturity() types.Scalar { return eq.maturity }

// Acceleration is an alias for RenewalRate adhering to the equation taxonomy.
type Acceleration = RenewalRate

func NewAcceleration() *Acceleration {
	return NewRenewalRate()
}
