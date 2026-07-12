package manifold

import "math"

const standardNormal95 = 1.959963984540054

/*
CalibrationSnapshot reports walk-forward error against a zero-return baseline.
Positive incremental MSE means the model reduced squared error.
*/
type CalibrationSnapshot struct {
	Samples        uint64
	ResidualStdDev float64
	IncrementalMSE float64
	LowerBound     float64
	Calibrated     bool
}

/*
Calibration owns prequential forecast errors. Every prediction is scored before
its target is used to update the model.
*/
type Calibration struct {
	samples       uint64
	residualMean  float64
	residualM2    float64
	advantageMean float64
	advantageM2   float64
}

/*
Observe scores one prior prediction against its newly observed target.
*/
func (calibration *Calibration) Observe(predicted float64, actual float64) {
	residual := actual - predicted
	advantage := actual*actual - residual*residual
	calibration.samples++
	calibration.updateResidual(residual)
	calibration.updateAdvantage(advantage)
}

func (calibration *Calibration) Snapshot(dimension int) CalibrationSnapshot {
	snapshot := CalibrationSnapshot{
		Samples:        calibration.samples,
		ResidualStdDev: calibration.deviation(calibration.residualM2),
		IncrementalMSE: calibration.advantageMean,
	}

	if calibration.samples < 2 {
		return snapshot
	}

	standardError := calibration.deviation(calibration.advantageM2) /
		math.Sqrt(float64(calibration.samples))
	snapshot.LowerBound = snapshot.IncrementalMSE - standardNormal95*standardError
	snapshot.Calibrated = calibration.samples > uint64(dimension+1) &&
		snapshot.LowerBound > 0

	return snapshot
}

func (calibration *Calibration) Confidence(predicted float64) float64 {
	deviation := calibration.deviation(calibration.residualM2)

	if calibration.samples < 2 || deviation <= 0 {
		return 0
	}

	zScore := math.Abs(predicted) / deviation
	return 0.5 * (1 + math.Erf(zScore/math.Sqrt2))
}

func (calibration *Calibration) updateResidual(value float64) {
	delta := value - calibration.residualMean
	calibration.residualMean += delta / float64(calibration.samples)
	calibration.residualM2 += delta * (value - calibration.residualMean)
}

func (calibration *Calibration) updateAdvantage(value float64) {
	delta := value - calibration.advantageMean
	calibration.advantageMean += delta / float64(calibration.samples)
	calibration.advantageM2 += delta * (value - calibration.advantageMean)
}

func (calibration *Calibration) deviation(sumSquares float64) float64 {
	if calibration.samples < 2 {
		return 0
	}

	return math.Sqrt(sumSquares / float64(calibration.samples-1))
}
