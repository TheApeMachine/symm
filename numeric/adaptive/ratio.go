package adaptive

import (
	"fmt"
	"math"
)

/*
Ratio expresses one signal relative to another. This
is the fundamental way to combine two measurements
without introducing a constant. Instead of scaling a
signal by some magic multiplier, you divide it by
another observed signal. The result is dimensionless
and self-scaling.

Ratio expects exactly two values per call: the
numerator and the denominator. A zero denominator
returns an error from Next (see Ratio.Next) so callers
must handle that case explicitly.
*/
type Ratio struct {
	raw      float64
	smoother *EMA
}

/*
NewRatio creates a new Ratio. The output is smoothed
by an EMA that bootstraps itself from the observed
ratios.
*/
func NewRatio(raw float64) *Ratio {
	return &Ratio{raw: raw, smoother: NewEMA(raw)}
}

/*
Next accepts two values — numerator and denominator —
and returns the smoothed ratio. The denominator must
be the second value.
*/
func (ratio *Ratio) Next(
	out float64, values ...float64,
) (result float64, err error) {
	if len(values) != 2 {
		return 0, fmt.Errorf(
			"adaptive: Ratio.Next expects exactly numerator and denominator, got %d values",
			len(values),
		)
	}

	numerator := values[0]
	denominator := values[1]

	if denominator == 0 {
		return 0, fmt.Errorf("adaptive: Ratio.Next zero denominator")
	}

	ratio.raw = numerator / denominator

	return ratio.smoother.Next(out, ratio.raw)
}

/*
Reset clears the Ratio back to its initial state.
*/
func (ratio *Ratio) Reset() error {
	ratio.raw = math.NaN()

	return ratio.smoother.Reset()
}
