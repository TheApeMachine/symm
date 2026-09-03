package adaptive

import (
	"math"
)

/*
WindowType defines the adaptive window horizon strategy.
*/
type WindowType int

const (
	ADWIN WindowType = iota
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
type Window struct {
	Type WindowType

	// Internal state
	count        int
	capacity     int
	mean         float64
	variance     float64
	previousMean float64
	stability    float64
	welford      WelfordEngine
}

// Step observes a new sample and computes the emergent window capacity.
func (window *Window) Step(value float64) int {
	window.count++
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

// Capacity returns the current emergent window capacity.
func (window *Window) Capacity() int {
	if window.capacity < MinimumStatisticalSampleSize {
		return MinimumStatisticalSampleSize
	}

	return window.capacity
}

func (window *Window) stepADWIN(value float64) {
	window.capacity++

	if window.count < MinimumObservationsForDriftTest {
		window.previousMean = window.mean
		return
	}

	scale := math.Sqrt(window.variance)

	if scale > 0 {
		// Bifet & Gavalda (2007) Theorem 1: drift bound derived from empirical scale and sample size
		epsilon := scale * math.Sqrt(2.0*math.Log(float64(window.count)*AdwinConfidenceDenominator)/float64(window.capacity))
		drift := math.Abs(value - window.mean)

		// Statistically significant regime break: shed obsolete subwindow
		if drift > epsilon && window.capacity > MinimumStatisticalSampleSize {
			window.capacity = int(float64(window.capacity) / 2.0)

			if window.capacity < MinimumStatisticalSampleSize {
				window.capacity = MinimumStatisticalSampleSize
			}
		}
	}

	window.previousMean = window.mean
}

func (window *Window) stepStabilityGovernor(value float64) {
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

func (window *Window) stepKish(value float64) {
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
