package market

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
