package core

import (
	"math"
	"time"
)

/*
Tolerance is the largest relative uncertainty a scale or interval may carry
and still divide an observation. Every definedness rule below is derived from
it rather than from a count or a duration picked on its own.
*/
const Tolerance = 0.25

/*
MinimumPrior is the number of prior observations a sample standard deviation
needs before it may scale a z-score. The relative standard error of a sample
sigma from n observations is about 1/sqrt(2(n-1)); holding that to Tolerance
gives n >= 1 + 1/(2*Tolerance^2), which is 9 at a tolerance of 0.25.
*/
var MinimumPrior = math.Ceil(Unit + Unit/(2*Tolerance*Tolerance))

/*
Resolution is the square root of the float64 machine epsilon. A variance below
eps*magnitude^2 is smaller than the rounding error of squaring numbers of that
magnitude, so a scale below Resolution*magnitude cannot be told apart from
zero, and neither can a divisor that small next to its numerator.
*/
var Resolution = math.Sqrt(math.Nextafter(Unit, 2) - Unit)

/*
ClockResolution is the resolution of venue timestamps. Kraken prints trade and
book times in whole microseconds; every inter-trade interval stored for epoch
1791596128450467000 (23,667 of them) is a whole number of microseconds.
*/
const ClockResolution = time.Microsecond

/*
Negligible reports whether scale is indistinguishable from zero next to the
largest of magnitudes: at or below Resolution times it. The rule is relative
only; an absolute floor would refuse genuine dispersion in a unit that makes
every value small, such as micro-cap prices.
*/
func Negligible(scale float64, magnitudes ...float64) bool {
	reference := 0.0

	for _, magnitude := range magnitudes {
		reference = math.Max(reference, math.Abs(magnitude))
	}

	return math.Abs(scale) <= Resolution*reference
}

/*
PriorScale returns the sample standard deviation of count prior observations
with sum of squared deviations m2, when it may scale value against center: at
least MinimumPrior observations, positive dispersion, and a scale that is not
Negligible next to value or center.
*/
func PriorScale(count, m2, value, center float64) (float64, bool) {
	if count < MinimumPrior || m2 <= 0 {
		return 0, false
	}

	scale := math.Sqrt(m2 / (count - Unit))

	if Negligible(scale, value, center) {
		return 0, false
	}

	return scale, true
}

/*
Resolvable reports whether an interval of seconds is long enough to divide by:
the timestamp quantization, ClockResolution, is at most Tolerance of it. A
shorter interval separates two events the venue clock barely tells apart, and
a rate over it reports the clock's grain, not the market.
*/
func Resolvable(seconds float64) bool {
	return seconds > 0 && ClockResolution.Seconds() <= Tolerance*seconds
}
