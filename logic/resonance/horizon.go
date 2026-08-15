package resonance

import (
	"math"

	"github.com/theapemachine/nomagique/learning"
	"gonum.org/v1/gonum/stat/distuv"
)

/*
horizonLedger retains direction-skill statistics for every forecast horizon.
Reach is the largest contiguous horizon whose issued direction calls beat a
coin flip at the regulated confidence. The next horizon stays a live probe, so
the model chooses how far it will stand behind a call.
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
observe records one issued direction call against the realized move over that
horizon. A zero forecast or a flat realization is not a call and is ignored.
*/
func (ledger *horizonLedger) observe(
	horizon int,
	forecast float64,
	actual float64,
) {
	if horizon < 1 {
		panic("resonance: positive forecast horizon required")
	}

	forecastDirection := signedDirection(forecast)
	actualDirection := signedDirection(actual)

	if forecastDirection == 0 || actualDirection == 0 {
		return
	}

	advantage := forecastDirection * actualDirection
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

func signedDirection(value float64) float64 {
	if value > 0 {
		return 1
	}

	if value < 0 {
		return -1
	}

	return 0
}

type directionEvidence struct {
	candidate  float64
	confidence float64
}

type stabilizedDirection struct {
	candidate       float64
	call            float64
	stable          float64
	held            bool
	confidence      float64
	switchThreshold float64
}

func directionPosterior(output learning.RLSOutput) directionEvidence {
	if !output.Ready || output.Scale <= 0 || output.DegreesOfFreedom <= 0 {
		return directionEvidence{}
	}

	candidate := signedDirection(output.Value)

	if candidate == 0 {
		return directionEvidence{}
	}

	distribution := distuv.StudentsT{
		Mu:    output.Value,
		Sigma: output.Scale,
		Nu:    output.DegreesOfFreedom,
	}
	positiveProbability := 1 - distribution.CDF(0)

	return directionEvidence{
		candidate:  candidate,
		confidence: max(positiveProbability, 1-positiveProbability),
	}
}

/*
stabilizeDirection applies a Schmitt-style posterior gate without turning a
retained direction into a trade. A same-side call remains actionable only while
it clears the ordinary admission confidence. Reversal uses the caller-supplied
switch boundary; otherwise the old state is retained for continuity while Call
is zero, so strategy cannot act on stale conviction.
*/
func stabilizeDirection(
	stable float64,
	evidence directionEvidence,
	entryThreshold float64,
	switchThreshold float64,
) stabilizedDirection {
	if switchThreshold < entryThreshold {
		switchThreshold = entryThreshold
	}

	result := stabilizedDirection{
		candidate:       evidence.candidate,
		stable:          signedDirection(stable),
		confidence:      evidence.confidence,
		switchThreshold: switchThreshold,
	}

	if evidence.candidate == 0 || evidence.confidence < entryThreshold {
		result.held = result.stable != 0
		return result
	}

	if result.stable == 0 || evidence.candidate == result.stable {
		result.call = evidence.candidate
		result.stable = evidence.candidate
		return result
	}

	if evidence.confidence >= switchThreshold {
		result.call = evidence.candidate
		result.stable = evidence.candidate
		return result
	}

	result.held = true
	return result
}

func directionCall(output learning.RLSOutput, minimumConfidence float64) float64 {
	return stabilizeDirection(
		0,
		directionPosterior(output),
		minimumConfidence,
		minimumConfidence,
	).call
}
