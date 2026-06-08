package numeric

import "fmt"

/*
FastSlowRatio compares the mean rate in the trailing fast window to the mean
rate in the preceding slow window. A zero slow baseline is smoothed by
recentRate * epsilon so breakouts after silence produce a high ratio.
*/
type FastSlowRatio struct {
	fastWindow int
	epsilon    float64
	invert     bool
}

func NewFastSlowRatio(fastWindow int, epsilon float64) *FastSlowRatio {
	if fastWindow <= 0 {
		fastWindow = 3
	}

	return &FastSlowRatio{
		fastWindow: fastWindow,
		epsilon:    epsilon,
	}
}

/*
NewInvertedFastSlowRatio returns older/recent for compression-style metrics.
*/
func NewInvertedFastSlowRatio(fastWindow int, epsilon float64) *FastSlowRatio {
	ratio := NewFastSlowRatio(fastWindow, epsilon)
	ratio.invert = true

	return ratio
}

func (ratio *FastSlowRatio) Next(
	out float64, values ...float64,
) (float64, error) {
	_ = out

	for _, sample := range values {
		if sample < 0 {
			return 0, fmt.Errorf("numeric: FastSlowRatio negative sample")
		}
	}

	if ratio.invert {
		return InvertedFastSlowRate(values, ratio.fastWindow, ratio.epsilon), nil
	}

	return FastSlowRate(values, ratio.fastWindow, ratio.epsilon), nil
}

func (ratio *FastSlowRatio) Reset() error {
	return nil
}

func FastSlowRate(samples []float64, fastWindow int, epsilon float64) float64 {
	sampleCount := len(samples)

	if fastWindow <= 0 {
		fastWindow = 3
	}

	if sampleCount <= fastWindow {
		return 1.0
	}

	slowCount := sampleCount - fastWindow
	recentRate := Mean(samples[sampleCount-fastWindow:])

	if slowCount <= 0 {
		return 1.0
	}

	olderRate := Mean(samples[:slowCount])

	if olderRate <= 0 {
		olderRate = recentRate * epsilon

		if olderRate <= 0 {
			return 1.0
		}
	}

	return recentRate / olderRate
}

func InvertedFastSlowRate(samples []float64, fastWindow int, epsilon float64) float64 {
	sampleCount := len(samples)

	if fastWindow <= 0 {
		fastWindow = 3
	}

	if sampleCount <= fastWindow {
		return 0.0
	}

	slowCount := sampleCount - fastWindow
	recentRate := Mean(samples[sampleCount-fastWindow:])

	if recentRate <= 0 {
		return 0.0
	}

	if slowCount <= 0 {
		return 0.0
	}

	olderRate := Mean(samples[:slowCount])

	if olderRate <= 0 {
		olderRate = recentRate * epsilon
	}

	return olderRate / recentRate
}

func (ratio *FastSlowRatio) FastWindow() int {
	return ratio.fastWindow
}

func (ratio *FastSlowRatio) Epsilon() float64 {
	return ratio.epsilon
}

func (ratio *FastSlowRatio) SetEpsilon(epsilon float64) error {
	if epsilon <= 0 {
		return fmt.Errorf("numeric: FastSlowRatio epsilon must be positive")
	}

	ratio.epsilon = epsilon

	return nil
}
