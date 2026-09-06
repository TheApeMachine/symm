package tests

import "math"

// MinimumSampleCountForVariance is the degrees of freedom threshold (n >= 2).
const MinimumSampleCountForVariance = 2.0

/*
referenceMoments provides single-pass, numerically stable running sample variance
and mean updates according to B. P. Welford (1962, Technometrics).
Computes sample variance with n - 1 degrees of freedom (unbiased Bessel correction).
Zero heap allocations.
*/
type referenceMoments struct {
	count float64
	mean  float64
	m2    float64
}

// Update incorporates a new sample into the running moments and returns (mean, stdDev).
func (welford *referenceMoments) Update(value float64) (float64, float64) {
	welford.count++
	delta := value - welford.mean
	welford.mean += delta / welford.count
	delta2 := value - welford.mean
	welford.m2 += delta * delta2

	if welford.count < MinimumSampleCountForVariance {
		return welford.mean, 0
	}

	variance := welford.m2 / (welford.count - 1.0)

	return welford.mean, math.Sqrt(variance)
}

func (welford *referenceMoments) Mean() float64 {
	return welford.mean
}

func (welford *referenceMoments) Dispersion() float64 {
	if welford.count < MinimumSampleCountForVariance {
		return 0
	}

	return math.Sqrt(welford.m2 / (welford.count - 1.0))
}

func (welford *referenceMoments) Variance() float64 {
	if welford.count < MinimumSampleCountForVariance {
		return 0
	}

	return welford.m2 / (welford.count - 1.0)
}

func (welford *referenceMoments) Count() float64 {
	return welford.count
}

/*
Shed scales the accumulated sample mass by retain, holding the mean fixed.

An estimator that never forgets converges on the pooled moments of every
regime it has ever seen: its dispersion grows to span the differences between
regimes, and its mean stops moving because each new sample carries weight
1/count. Both make a departure from the current regime unmeasurable, which is
the opposite of what a baseline exists to do.

Shedding scales count and the sum of squared deviations by the same factor, so
the mean and the dispersion are both preserved at the moment of the call while
the support behind them collapses. Nothing is discarded discontinuously: the
estimator keeps its current best statement of the level and simply becomes
willing to be moved off it again.
*/
func (welford *referenceMoments) Shed(retain float64) {
	if retain >= 1 || retain <= 0 || welford.count <= MinimumSampleCountForVariance {
		return
	}

	shed := welford.count * retain

	if shed < MinimumSampleCountForVariance {
		shed = MinimumSampleCountForVariance
	}

	// Dispersion is the Bessel-corrected sample deviation sqrt(m2/(n-1)), so
	// holding it fixed across the shed means scaling m2 by the ratio of the
	// corrected degrees of freedom, not by the ratio of the raw counts.
	welford.m2 *= (shed - 1) / (welford.count - 1)
	welford.count = shed
}

/*
referenceWindowType defines the adaptive window horizon strategy.
*/
type referenceWindowType int

const (
	ADWIN referenceWindowType = iota
	STABILITY_GOV
	KISH
)

// MinimumStatisticalSampleSize is the minimal sample count for sample variance (degrees of freedom n - 1 >= 1).
const MinimumStatisticalSampleSize = 2

// MinimumObservationsForDriftTest is the minimum sample size required to form two comparison subwindows (degrees of freedom 2 + 2 = 4).
const MinimumObservationsForDriftTest = 4

// AdwinConfidenceDenominator = 4.0 from Bifet & Gavalda (2007 Theorem 1):
// epsilon_cut = sqrt((1/(2m)) * sigma^2 * ln(4n / delta)) with delta = 1/n.
const AdwinConfidenceDenominator = 4.0

// PerfectStabilityThreshold = 1.0 represents zero instantaneous deviation from running mean.
const PerfectStabilityThreshold = 1.0

/*
Window maintains an adaptive lookback horizon whose capacity expands when the data
stream is stationary and contracts causally when statistical change or concept drift
is detected.
Zero magic numbers: parameters are derived causally from the data itself.
*/
type referenceWindow struct {
	Type referenceWindowType

	// Internal state
	count        int
	capacity     int
	mean         float64
	variance     float64
	previousMean float64
	stability    float64
	shedRatio    float64
	welford      referenceMoments
	recent       referenceMoments
}

// Step observes a new sample and computes the emergent window capacity.
func (window *referenceWindow) Step(value float64) int {
	window.count++
	window.shedRatio = 1
	mean, stdDev := window.welford.Update(value)
	window.mean = mean
	window.variance = stdDev * stdDev

	if window.capacity == 0 {
		window.capacity = MinimumStatisticalSampleSize
	}

	switch window.Type {
	case ADWIN:
		window.stepADWIN(value)
	case STABILITY_GOV:
		window.stepStabilityGovernor(value)
	case KISH:
		window.stepKish(value)
	default:
		window.stepADWIN(value)
	}

	return window.capacity
}

/*
ShedRatio reports the fraction of accumulated support the last Step retained:
1 when no regime break was detected, and the contraction factor when one was.
A caller keeping its own moments alongside this window shrinks them by the same
factor to stay on the window's horizon.
*/
func (window *referenceWindow) ShedRatio() float64 {
	if window.shedRatio <= 0 {
		return 1
	}

	return window.shedRatio
}

// Capacity returns the current emergent window capacity.
func (window *referenceWindow) Capacity() int {
	if window.capacity < MinimumStatisticalSampleSize {
		return MinimumStatisticalSampleSize
	}

	return window.capacity
}

/*
stepADWIN applies the Bifet & Gavalda (2007) Theorem 1 cut: the window is
contracted when the mean of a recent subwindow departs from the mean of the
whole window by more than the bound their theorem places on that difference
under the hypothesis that both halves were drawn from one distribution.

The comparison must be between two subwindow MEANS. Testing the newest single
observation against the running mean instead is not a drift test at all: one
sample from a stationary stream sits beyond one standard deviation about a
third of the time, so such a rule contracts continuously on quiet data and
destroys the very support the window exists to accumulate. Averaging the
recent subwindow is what suppresses that per-sample noise, and it is why the
bound below scales with the harmonic mean of the two subwindow sizes rather
than with a single capacity.
*/
func (window *referenceWindow) stepADWIN(value float64) {
	window.capacity++
	window.recent.Update(value)

	if window.count < MinimumObservationsForDriftTest {
		window.previousMean = window.mean
		return
	}

	// The recent subwindow is held to half the window it is tested against, so
	// neither side of the comparison can shrink to a point.
	if window.recent.Count() > float64(window.capacity)/2.0 {
		window.recent.Shed(0.5)
	}

	recentCount := window.recent.Count()
	priorCount := float64(window.capacity) - recentCount

	if recentCount < MinimumStatisticalSampleSize || priorCount < MinimumStatisticalSampleSize {
		window.previousMean = window.mean
		return
	}

	scale := math.Sqrt(window.variance)

	if scale > 0 {
		// Harmonic mean of the two subwindow sizes, per Theorem 1.
		harmonic := 1.0 / (1.0/recentCount + 1.0/priorCount)

		// delta = 1/n gives ln(4n/delta) = ln(4n^2).
		count := float64(window.count)
		epsilon := scale * math.Sqrt(
			math.Log(AdwinConfidenceDenominator*count*count)/(2.0*harmonic),
		)
		drift := math.Abs(window.recent.Mean() - window.mean)

		// Statistically significant regime break: shed obsolete subwindow
		if drift > epsilon && window.capacity > MinimumStatisticalSampleSize {
			previousCapacity := window.capacity
			window.capacity = int(float64(window.capacity) / 2.0)

			if window.capacity < MinimumStatisticalSampleSize {
				window.capacity = MinimumStatisticalSampleSize
			}

			window.shedRatio = float64(window.capacity) / float64(previousCapacity)

			// The drift bound is derived from this window's own scale, so the
			// detector must forget on the same schedule as the window it
			// governs. Left cumulative, its epsilon widens with every regime
			// it has ever seen until no break can clear it, and the window
			// stops contracting exactly when the stream is least stationary.
			window.welford.Shed(window.shedRatio)

			// The recent subwindow established the break; it becomes the basis
			// the next one is measured against.
			window.recent = referenceMoments{}
		}
	}

	window.previousMean = window.mean
}

func (window *referenceWindow) stepStabilityGovernor(value float64) {
	if window.count < MinimumStatisticalSampleSize {
		window.capacity = MinimumStatisticalSampleSize
		return
	}

	// Directional stability S = 1 / (1 + |x - mu|/sigma)
	delta := math.Abs(value - window.mean)
	spread := math.Sqrt(window.variance)

	currentStability := PerfectStabilityThreshold

	if spread > 0 {
		currentStability = 1.0 / (1.0 + delta/spread)
	}

	// Expand when stability drops (need more evidence), contract when stable
	if currentStability < window.stability {
		// Stability declined: expand window capacity
		window.capacity = window.capacity + window.capacity
	} else if currentStability >= PerfectStabilityThreshold && window.capacity > MinimumStatisticalSampleSize {
		// Perfect stability: contract capacity toward current sample evidence
		window.capacity = window.count
	}

	window.stability = currentStability
}

func (window *referenceWindow) stepKish(value float64) {
	if window.variance <= 0 || window.mean == 0 {
		window.capacity = window.count

		return
	}

	// Kish's effective sample size: n_eff = n / (1 + CV^2) (Kish, 1965)
	cv := math.Sqrt(window.variance) / math.Abs(window.mean)
	effectiveSize := float64(window.count) / (1.0 + cv*cv)

	if effectiveSize < MinimumStatisticalSampleSize {
		effectiveSize = MinimumStatisticalSampleSize
	}

	window.capacity = int(effectiveSize)
}
