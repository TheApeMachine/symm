package logic

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
QualifiesForOpportunityEntry reports whether the best coherent candidate exceeds
the elevated confidence and surprise bars and carries a high-value category.
*/
func QualifiesForOpportunityEntry(
	measurements []Measurement,
	thresholdCtx ThresholdContext,
) bool {
	costBps := ExecutionCostFromMeasurements(measurements, 0, 0, 0)
	candidate, ok := BestEntryCandidate(measurements, costBps)

	if !ok {
		return false
	}

	return QualifiesForOpportunityEntryFromCandidate(candidate, thresholdCtx)
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
