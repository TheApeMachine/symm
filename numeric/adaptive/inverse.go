package adaptive

import (
	"fmt"
	"sync"
)

/*
Inverse produces the counter-signal. High input yields
low output, low input yields high output. Critically,
it does not use 1/x with some arbitrary floor — it
tracks the observed min and max of the signal and
mirrors the value within that range.

If the signal has ranged from 2 to 10, and the current
value is 8, the inverse is 4. The reflection point is
derived entirely from what the signal has actually done.

This is Newton's third law as a dynamic: every signal
has an equal and opposite reaction, bounded by its own
observed behavior.
*/
type Inverse struct {
	mu       sync.Mutex
	min      float64
	max      float64
	observed bool
}

/*
NewInverse creates a new Inverse. The range is learned
from observations — no bounds need to be specified.
*/
func NewInverse() *Inverse {
	return &Inverse{}
}

/*
Next accepts a signal value and returns its mirror
within the observed range. On the first observation
it returns the value unchanged, since there is no
range to invert within yet.
*/
func (inverse *Inverse) Next(
	out float64, values ...float64,
) (float64, error) {
	_ = out

	if len(values) == 0 {
		return 0, fmt.Errorf("adaptive: Inverse.Next requires at least one value")
	}

	inverse.mu.Lock()
	defer inverse.mu.Unlock()

	var result float64

	for _, observation := range values {
		if !inverse.observed {
			inverse.min = observation
			inverse.max = observation
			inverse.observed = true
			result = observation

			continue
		}

		if observation < inverse.min {
			inverse.min = observation
		}

		if observation > inverse.max {
			inverse.max = observation
		}

		// Mirror the value within the observed range.
		// min + max - observation reflects around the
		// midpoint of [min, max].
		result = inverse.min + inverse.max - observation
	}

	return result, nil
}

/*
Reset clears the Inverse back to its unobserved state.
*/
func (inverse *Inverse) Reset() error {
	inverse.mu.Lock()
	defer inverse.mu.Unlock()

	inverse.min = 0
	inverse.max = 0
	inverse.observed = false

	return nil
}
