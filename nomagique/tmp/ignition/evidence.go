package equation

import (
	"math"
)

// IgnitionSideOutput contains one reciprocal price direction's volume-clock
// families. Buy scores positive log returns; Sell scores the same observations
// under reciprocal price, which is exactly the negative original log return.
type IgnitionSideOutput struct {
	Value       float64
	RVOL        float64
	Precursor   float64
	Compression float64
	Ignition    float64
	Trend       float64
}

package equation

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/statistic"
)

/*
ignitionFamilies combines empirically normalized lift, price, and spread
evidence into the three active pump families.
*/
func ignitionFamilies(
	rvol float64,
	precursor float64,
	compression float64,
	rvolScale float64,
	precursorScale float64,
	compressionScale float64,
) (IgnitionSideOutput, error) {
	scaledRVOL := ignitionSquash(rvol, rvolScale)
	quietCompression := ignitionInverse(compression, compressionScale)
	quietPrecursor := ignitionInverse(precursor, precursorScale)
	ignitionScore, err := ignitionMean(rvol > 0 && precursor > 0, rvol, precursor)

	if err != nil {
		return IgnitionSideOutput{}, err
	}

	trendScore, err := ignitionMean(
		precursor > 0 && scaledRVOL > 0 && quietCompression > 0,
		precursor,
		scaledRVOL,
		quietCompression,
	)

	if err != nil {
		return IgnitionSideOutput{}, err
	}

	compressionScore, err := ignitionMean(
		compression > 0 && scaledRVOL > 0 && quietPrecursor > 0,
		compression,
		scaledRVOL,
		quietPrecursor,
	)

	if err != nil {
		return IgnitionSideOutput{}, err
	}

	return IgnitionSideOutput{
		RVOL:        rvol,
		Precursor:   precursor,
		Compression: compressionScore,
		Ignition:    ignitionScore,
		Trend:       trendScore,
	}, nil
}

/*
ignitionExhaustion requires both declining relative lift and empirically scaled
price rejection, so high-volume continuation cannot masquerade as exhaustion.
*/
func ignitionExhaustion(
	priorRVOL float64,
	rvol float64,
	rejection float64,
	moveScale float64,
) float64 {
	if priorRVOL <= 0 || rejection <= 0 || moveScale <= 0 {
		return 0
	}

	return (math.Max(0, priorRVOL-rvol) / priorRVOL) *
		ignitionSquash(rejection, moveScale)
}

/*
ignitionRatio normalizes evidence only when its empirical baseline is ready.
*/
func ignitionRatio(value, baseline float64, ready bool) float64 {
	if !ready || baseline <= 0 || value <= 0 {
		return 0
	}

	return value / baseline
}

/*
ignitionMean combines ready evidence and returns probability calculation errors.
*/
func ignitionMean(ready bool, values ...float64) (float64, error) {
	if !ready {
		return 0, nil
	}

	score, err := statistic.EvidenceGeomean(values...)

	if err != nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"ignition: combine empirical evidence",
			err,
		))
	}

	return score, nil
}

/*
ignitionSquash maps positive evidence through an empirical positive scale.
*/
func ignitionSquash(value float64, scale float64) float64 {
	if value <= 0 || scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		return 0
	}

	return value / (scale + value)
}

/*
ignitionInverse maps counter-evidence through an empirical scale. Measured
absence is complete quiet and therefore needs no positive-event scale.
*/
func ignitionInverse(value float64, scale float64) float64 {
	if value < 0 {
		return 0
	}

	if value == 0 {
		return 1
	}

	if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		return 0
	}

	return scale / (scale + value)
}

/*
ignitionRatioScale derives the typical retained ratio against its own baseline.
*/
func ignitionRatioScale(values []float64, baseline float64) float64 {
	if baseline <= 0 || len(values) == 0 {
		return 0
	}

	median, ready := statistic.MedianOf(values)

	if !ready || median <= 0 || math.IsNaN(median) || math.IsInf(median, 0) {
		return 0
	}

	return median / baseline
}

/*
ignitionCompressionScale derives prior positive compression without inventing
a scale when the retained spread tape contains none.
*/
func ignitionCompressionScale(spreads []float64, baseline float64) float64 {
	if baseline <= 0 || len(spreads) == 0 {
		return 0
	}

	compressions := make([]float64, 0, len(spreads)/2)

	for _, spread := range spreads {
		if spread <= 0 || math.IsNaN(spread) || math.IsInf(spread, 0) {
			continue
		}

		compression := 1 - spread/baseline

		if compression > 0 {
			compressions = append(compressions, compression)
		}
	}

	median, ready := statistic.MedianOf(compressions)

	if !ready || median <= 0 || math.IsNaN(median) || math.IsInf(median, 0) {
		return 0
	}

	return median
}
