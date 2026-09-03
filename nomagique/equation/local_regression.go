package equation

import (
	"math"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

type regressionPoint struct {
	timestamp int64
	value     float64
}

/*
LocalRegression fits a causal ordinary least squares linear regression over an
adaptive event-time horizon. It evaluates regression slope, variance, and
SNR without static time windows or magic lookback spans.
*/
type LocalRegression struct {
	samples []regressionPoint

	slope            float64
	intercept        float64
	slopeVariance    float64
	slopeSNR         float64
	residualVariance float64
	hasFit           bool
	hasSNR           bool
}

/*
Step fits the regression on all prior samples within the horizon,
evaluates slope and SNR, and retains the new sample for subsequent steps.
Returns the fitted slope as Number.
*/
func (regression *LocalRegression) Step(value float64, currentNano int64, horizonSeconds float64) nmtypes.Number {
	cutoffNano := int64(0)

	if horizonSeconds > 0 {
		cutoffNano = currentNano - int64(horizonSeconds*1e9)
	}

	var inWindow []regressionPoint

	for _, sample := range regression.samples {
		if cutoffNano <= 0 || sample.timestamp >= cutoffNano {
			inWindow = append(inWindow, sample)
		}
	}

	regression.fit(inWindow, currentNano)

	// Append current observation after fitting
	regression.samples = append(regression.samples, regressionPoint{
		timestamp: currentNano,
		value:     value,
	})

	return nmtypes.Number(regression.slope)
}

func (regression *LocalRegression) fit(samples []regressionPoint, currentNano int64) {
	sampleCount := float64(len(samples))

	if sampleCount < 2 {
		regression.hasFit = false
		regression.hasSNR = false
		regression.slope = 0
		regression.slopeSNR = 0

		return
	}

	sumTau := 0.0
	sumX := 0.0

	for _, sample := range samples {
		tau := float64(sample.timestamp-currentNano) / 1e9
		sumTau += tau
		sumX += sample.value
	}

	meanTau := sumTau / sampleCount
	meanX := sumX / sampleCount

	sumTauTau := 0.0
	sumTauX := 0.0
	sumXX := 0.0

	for _, sample := range samples {
		tau := float64(sample.timestamp-currentNano) / 1e9
		deltaTau := tau - meanTau
		deltaX := sample.value - meanX

		sumTauTau += deltaTau * deltaTau
		sumTauX += deltaTau * deltaX
		sumXX += deltaX * deltaX
	}

	if sumTauTau <= 0 || math.IsNaN(sumTauTau) {
		regression.hasFit = false
		regression.hasSNR = false

		return
	}

	regression.slope = sumTauX / sumTauTau
	regression.intercept = meanX - regression.slope*meanTau

	sumSquaredErrors := sumXX - regression.slope*sumTauX

	if sumSquaredErrors < 0 {
		sumSquaredErrors = 0
	}

	degreesOfFreedom := sampleCount - 2
	regression.hasFit = true
	regression.hasSNR = false
	regression.slopeVariance = 0
	regression.slopeSNR = 0

	if degreesOfFreedom > 0 {
		regression.residualVariance = sumSquaredErrors / degreesOfFreedom
		regression.slopeVariance = regression.residualVariance / sumTauTau

		if regression.slopeVariance > 0 {
			regression.slopeSNR = (regression.slope * regression.slope) / regression.slopeVariance
			regression.hasSNR = true
		}
	}
}

func (regression *LocalRegression) Slope() (float64, bool) {
	return regression.slope, regression.hasFit
}

func (regression *LocalRegression) SNR() (float64, bool) {
	return regression.slopeSNR, regression.hasSNR
}

func (regression *LocalRegression) Variance() float64 {
	return regression.slopeVariance
}
