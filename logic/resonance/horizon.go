package resonance

import (
	"math"

	"gonum.org/v1/gonum/stat/distuv"
)

/*
horizonLedger retains sufficient loss statistics for every forecast horizon.
Reach is the largest contiguous horizon whose paired squared-error advantage
over a zero-return forecast clears the regulated confidence threshold.
*/
type horizonLedger struct {
	horizons map[int]*horizonStatistic
	resolved int
}

type horizonStatistic struct {
	count int
	mean  float64
	m2    float64
}

/*
newHorizonLedger creates an empty, uncapped horizon calibration ledger.
*/
func newHorizonLedger() *horizonLedger {
	return &horizonLedger{horizons: make(map[int]*horizonStatistic)}
}

/*
observe records one exact horizon forecast against its aligned realized return.
*/
func (ledger *horizonLedger) observe(
	horizon int,
	forecast float64,
	actual float64,
) {
	if horizon < 1 {
		panic("resonance: positive forecast horizon required")
	}

	modelError := actual - forecast
	advantage := actual*actual - modelError*modelError
	statistic := ledger.horizons[horizon]

	if statistic == nil {
		statistic = &horizonStatistic{}
		ledger.horizons[horizon] = statistic
	}

	statistic.count++
	delta := advantage - statistic.mean
	statistic.mean += delta / float64(statistic.count)
	statistic.m2 += delta * (advantage - statistic.mean)
	ledger.resolved++
}

/*
supported returns the contiguous calibrated reach at the supplied one-sided
confidence. The next horizon remains a live probe, so reach can grow without a
configured maximum.
*/
func (ledger *horizonLedger) supported(confidence float64) int {
	if confidence < 0.5 || confidence > 1 {
		panic("resonance: horizon confidence must be in [0.5,1]")
	}

	reach := 0

	for horizon := 1; ; horizon++ {
		statistic := ledger.horizons[horizon]

		if statistic == nil || statistic.confidence() < confidence {
			return reach
		}

		reach = horizon
	}
}

/*
confidence returns P(mean loss advantage > 0) under the paired Student-t
sampling distribution. Two observations are required to estimate variance.
*/
func (statistic *horizonStatistic) confidence() float64 {
	if statistic == nil || statistic.count < 2 {
		return 0
	}

	if statistic.m2 == 0 {
		if statistic.mean > 0 {
			return 1
		}

		if statistic.mean < 0 {
			return 0
		}

		return 0.5
	}

	standardError := math.Sqrt(
		statistic.m2 /
			float64(statistic.count-1) /
			float64(statistic.count),
	)
	distribution := distuv.StudentsT{
		Mu:    statistic.mean,
		Sigma: standardError,
		Nu:    float64(statistic.count - 1),
	}

	return 1 - distribution.CDF(0)
}
