package logic

/*
SurpriseThresholdFn resolves the live surprise bar for a source.
Market wires this at boot to avoid an import cycle with the registry.
*/
var SurpriseThresholdFn func(source SourceType) float64

func noveltyBarForCandidate(candidate EntryCandidate, thresholdCtx ThresholdContext) float64 {
	noveltyBar := thresholdCtx.DynamicSurpriseBaseline()

	if SurpriseThresholdFn == nil {
		return noveltyBar
	}

	for _, source := range candidate.Sources {
		sourceBar := SurpriseThresholdFn(source)

		if sourceBar > noveltyBar {
			noveltyBar = sourceBar
		}
	}

	return noveltyBar
}

func surpriseAnchorForCandidate(
	candidate EntryCandidate,
	thresholdCtx ThresholdContext,
) float64 {
	return noveltyBarForCandidate(candidate, thresholdCtx)
}

/*
SurpriseAnchorForCandidate exposes the live novelty bar for sizing callers.
*/
func SurpriseAnchorForCandidate(
	candidate EntryCandidate,
	thresholdCtx ThresholdContext,
) float64 {
	return surpriseAnchorForCandidate(candidate, thresholdCtx)
}
