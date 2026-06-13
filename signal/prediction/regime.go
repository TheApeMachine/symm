package prediction

import "github.com/theapemachine/symm/logic"

type predictionRegime struct {
	source   logic.SourceType
	category logic.CategoryType
	regime   logic.RegimeType
	ready    bool
}

func (regime predictionRegime) Shifted(current predictionRegime) bool {
	if !regime.ready || !current.ready {
		return true
	}

	return regime != current
}

func (regime predictionRegime) Panic() bool {
	if !regime.ready {
		return false
	}

	return regime.category == logic.CategoryLiquidityShock ||
		regime.category == logic.CategorySystemicBeta
}

func (signal *Signal) recordFeatureMeasurement(measurement logic.Measurement) {
	sourceIndex := featureSourceIndex(measurement.Source)

	if sourceIndex < 0 {
		return
	}

	signal.features[sourceIndex] = measurement.Confidence
	signal.featureCategories[sourceIndex] = measurement.Category
	signal.featureRegimes[sourceIndex] = measurement.Regime
}

func (signal *Signal) currentRegime() predictionRegime {
	current := predictionRegime{}
	strongest := 0.0

	for sourceIndex, source := range featureSources {
		if !isMacroFeatureSource(source) {
			continue
		}

		category := signal.featureCategories[sourceIndex]
		regime := signal.featureRegimes[sourceIndex]

		if category == logic.CategoryTypeNone && regime == logic.RegimeTypeNone {
			continue
		}

		confidence := signal.features[sourceIndex]

		if confidence <= strongest {
			continue
		}

		strongest = confidence
		current = predictionRegime{
			source:   source,
			category: category,
			regime:   regime,
			ready:    true,
		}
	}

	return current
}

func isMacroFeatureSource(source logic.SourceType) bool {
	switch source {
	case logic.SourceCausal,
		logic.SourceCorrelation,
		logic.SourceLiquidity,
		logic.SourceManifold,
		logic.SourceSentiment:
		return true
	default:
		return false
	}
}
