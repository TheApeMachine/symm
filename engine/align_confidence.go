package engine

import "math"

/*
AlignConfidence combines dimensionless indicator strengths from the current
measurement into (0, 1). Only indicators present in this reading contribute;
absent hallmarks reduce alignment but do not zero it unless nothing fired.
*/
func AlignConfidence(factors ...float64) float64 {
	product := 1.0
	count := 0

	for _, factor := range factors {
		if factor <= 0 {
			continue
		}

		product *= factor
		count++
	}

	if count == 0 {
		return 0
	}

	mean := math.Pow(product, 1.0/float64(count))

	return mean / (mean + 1)
}

/*
ConfidenceFromScore maps one bounded measurement in (0, 1] into display strength.
Used when the score itself is the reading, not one of several hallmarks.
*/
func ConfidenceFromScore(score float64) float64 {
	if score <= 0 {
		return 0
	}

	return 1 - math.Exp(-score)
}

/*
ProvisionalConfidence returns normalized strength when history exists,
otherwise maps raw score into (0, 1) so cold symbols still emit gauge signal.
*/
func ProvisionalConfidence(normalized, rawScore float64) float64 {
	if normalized > 0 {
		return normalized
	}

	return ConfidenceFromScore(rawScore)
}

/*
ExcessRatio maps values above unity into (0, 1): how far past the unit threshold.
*/
func ExcessRatio(value float64) float64 {
	if value <= 1 {
		return 0
	}

	return (value - 1) / value
}

/*
TrustCalibratedConfidence applies the top-down feedback signal to a
raw confidence reading: when a signal's track record (modelled by the
per-(source,symbol) learned.Forecast trust score) is good, raw
confidence is published as-is; when it is poor, confidence is damped
toward zero. This is what closes the feedback loop the spec
describes — prediction error → forecast.weight update → trust → next
measurement's confidence is scaled by that trust → the trader's
perspective uses a calibrated confidence → the next prediction is
informed by past accuracy.

Bootstrapping is deliberate: until the calibrator has accumulated
minSamples settled predictions, the raw confidence passes through
unchanged so the signal can keep emitting predictions and feedback
can accumulate at all. After minSamples the factor is blended
0.5 + 0.5*trust so even a chronically-wrong signal still publishes at
half strength rather than disappearing — disappearance would starve
the calibrator of fresh feedback and trap the signal in its hole.

trust is expected in [0,1]; values outside that range are clipped.
*/
func TrustCalibratedConfidence(raw, trust float64, samples, minSamples int) float64 {
	if raw <= 0 {
		return 0
	}

	if samples < minSamples {
		return raw
	}

	if trust < 0 {
		trust = 0
	}

	if trust > 1 {
		trust = 1
	}

	factor := 0.5 + 0.5*trust

	return raw * factor
}
