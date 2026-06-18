package market

import "math"

func (calibrator *forwardCalibrator) observe(forecastBps, realizedBps, alpha float64) {
	calibrator.mu.Lock()
	defer calibrator.mu.Unlock()

	errorBps := realizedBps - forecastBps
	calibrator.samples++

	delta := realizedBps - calibrator.meanReturn
	calibrator.meanReturn += delta / float64(calibrator.samples)
	delta2 := realizedBps - calibrator.meanReturn
	calibrator.m2Return += delta * delta2

	if calibrator.samples == 1 {
		calibrator.mse = errorBps * errorBps
		calibrator.bias = errorBps

		if forecastBps != 0 {
			calibrator.scale = realizedBps / forecastBps
			calibrator.slopeSeen = true
		}

		return
	}

	calibrator.mse += alpha * (errorBps*errorBps - calibrator.mse)
	calibrator.bias += alpha * (errorBps - calibrator.bias)

	if forecastBps == 0 {
		return
	}

	ratio := realizedBps / forecastBps

	if !calibrator.slopeSeen {
		calibrator.scale = ratio
		calibrator.slopeSeen = true

		return
	}

	calibrator.scale += alpha * (ratio - calibrator.scale)
}

func (calibrator *forwardCalibrator) calibrate(
	rawBps, confidence float64,
	minSamples int,
	significanceZ float64,
) (expectedBps, strengthScale, confidenceScale float64) {
	calibrator.mu.Lock()
	defer calibrator.mu.Unlock()

	expectedBps = rawBps
	strengthScale = 1
	confidenceScale = 1

	if calibrator.samples == 0 || !calibrator.slopeSeen {
		return expectedBps, strengthScale, confidenceScale
	}

	expectedBps = calibrator.scale*rawBps + calibrator.bias

	if !calibrator.warmLocked(minSamples, significanceZ) {
		return expectedBps, strengthScale, confidenceScale
	}

	strengthScale = math.Abs(calibrator.scale)

	if strengthScale <= 0 || math.IsNaN(strengthScale) || math.IsInf(strengthScale, 0) {
		strengthScale = 1
	}

	trust := calibrator.trustLocked(minSamples)

	if confidence > 0 {
		confidenceScale = trust
	}

	return expectedBps, strengthScale, confidenceScale
}

func (calibrator *forwardCalibrator) warmLocked(minSamples int, significanceZ float64) bool {
	if calibrator.samples < minSamples {
		return false
	}

	variance := 0.0

	if calibrator.samples > 1 {
		variance = calibrator.m2Return / float64(calibrator.samples-1)
	}

	stderr := math.Sqrt(variance / float64(calibrator.samples))

	return calibrator.meanReturn-significanceZ*stderr > 0
}

func (calibrator *forwardCalibrator) trustLocked(minSamples int) float64 {
	if calibrator.samples <= 0 || minSamples <= 0 {
		return 1
	}

	fullTrustAt := minSamples * 2

	trust := float64(calibrator.samples) / float64(fullTrustAt)

	if trust > 1 {
		return 1
	}

	if trust < 0 {
		return 0
	}

	return trust
}

func (calibrator *forwardCalibrator) snapshot(symbol string) *Feedback {
	calibrator.mu.Lock()
	defer calibrator.mu.Unlock()

	return NewFeedback(
		symbol,
		calibrator.mse,
		calibrator.scale,
		calibrator.bias,
		calibrator.samples,
	)
}
