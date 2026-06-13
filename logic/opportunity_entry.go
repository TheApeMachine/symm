package logic

import "github.com/theapemachine/symm/config"

/*
PeakSurprise returns the highest surprise across a measurement spectrum.
*/
func PeakSurprise(measurements []Measurement) float64 {
	peak := 0.0

	for _, measurement := range measurements {
		if measurement.Surprise > peak {
			peak = measurement.Surprise
		}
	}

	return peak
}

/*
PeakStrength returns the highest strength across a measurement spectrum.
*/
func PeakStrength(measurements []Measurement) float64 {
	peak := 0.0

	for _, measurement := range measurements {
		if measurement.Strength > peak {
			peak = measurement.Strength
		}
	}

	return peak
}

/*
QualifiesForOpportunityEntry reports whether a candidate exceeds the elevated
confidence and surprise bars and carries a high-value pump or catch-up category.
*/
func QualifiesForOpportunityEntry(
	measurements []Measurement,
	threshold config.ThresholdConfig,
) bool {
	confidence := PeakConfidence(measurements)
	surprise := PeakSurprise(measurements)
	strength := PeakStrength(measurements)

	if strength <= 0 {
		return false
	}

	confidenceBar := threshold.EntryConfidenceBaseline +
		threshold.TurbulenceConfidenceScale

	if confidence < confidenceBar {
		return false
	}

	if surprise < threshold.EntrySurpriseBaseline {
		return false
	}

	return hasHighValueOpportunityCategory(measurements)
}

func hasHighValueOpportunityCategory(measurements []Measurement) bool {
	for _, measurement := range measurements {
		if measurement.Source == SourcePumpDump {
			return true
		}

		switch measurement.Category {
		case CategoryFrenzy,
			CategoryVerticalIgnition,
			CategoryAggressiveDrive,
			CategoryRiskOnSurge,
			CategoryInefficientLag:
			return true
		default:
		}
	}

	return false
}
