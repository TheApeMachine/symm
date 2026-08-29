package hawkes

import "testing"

func TestClassifyFitRequiresReadyGates(testingT *testing.T) {
	fit := bivariateFit{muX: 1, muY: 1, beta: 1, intensityX: 1, intensityY: 1}

	if _, _, err := classifyFit(fit, 0, false, fitGates{}); err == nil {
		testingT.Fatal("expected an error when gates are not ready")
	}
}

func TestClassifyFitFrenzyAboveThreshold(testingT *testing.T) {
	gates := fitGates{saturationRadius: 0.9, frenzyAsymmetry: 0.5}
	fit := bivariateFit{muX: 1, muY: 1, beta: 1, spectralRadius: 0.1}

	category, confidence, err := classifyFit(fit, 0.8, false, gates)

	if err != nil {
		testingT.Fatalf("classifyFit: %v", err)
	}

	if category != fitCategoryFrenzy {
		testingT.Fatalf("expected fitCategoryFrenzy, got %v", category)
	}

	if confidence <= 0 || confidence > 1 {
		testingT.Fatalf("expected confidence in (0, 1], got %v", confidence)
	}
}

func TestClassifyFitSaturationAboveRadius(testingT *testing.T) {
	gates := fitGates{saturationRadius: 0.5, frenzyAsymmetry: 0.9}
	fit := bivariateFit{muX: 1, muY: 1, beta: 1, spectralRadius: 0.8, intensityX: 1, intensityY: 1}

	category, _, err := classifyFit(fit, 0.1, false, gates)

	if err != nil {
		testingT.Fatalf("classifyFit: %v", err)
	}

	if category != fitCategorySaturation {
		testingT.Fatalf("expected fitCategorySaturation, got %v", category)
	}
}

func TestClassifyFitExhaustionBelowBaseline(testingT *testing.T) {
	gates := fitGates{saturationRadius: 0.9, frenzyAsymmetry: 0.9}
	fit := bivariateFit{muX: 1, muY: 1, beta: 1, spectralRadius: 0.1, intensityX: 0.2, intensityY: 1}

	category, _, err := classifyFit(fit, 0.1, false, gates)

	if err != nil {
		testingT.Fatalf("classifyFit: %v", err)
	}

	if category != fitCategoryExhaustion {
		testingT.Fatalf("expected fitCategoryExhaustion, got %v", category)
	}
}

func TestClassifyFitOrganicDefault(testingT *testing.T) {
	gates := fitGates{saturationRadius: 0.9, frenzyAsymmetry: 0.9}
	fit := bivariateFit{muX: 1, muY: 1, beta: 1, spectralRadius: 0.1, intensityX: 1, intensityY: 1}

	category, confidence, err := classifyFit(fit, 0.1, false, gates)

	if err != nil {
		testingT.Fatalf("classifyFit: %v", err)
	}

	if category != fitCategoryOrganic {
		testingT.Fatalf("expected fitCategoryOrganic, got %v", category)
	}

	if confidence <= 0 {
		testingT.Fatalf("expected positive confidence, got %v", confidence)
	}
}

func TestFitGatesFromHistoryRequiresLongWindow(testingT *testing.T) {
	if _, ok := fitGatesFromHistory(nil, nil); ok {
		testingT.Fatal("expected empty history to fail gate derivation")
	}
}

func TestFitGatesFromHistoryDerivesPositiveGates(testingT *testing.T) {
	radii := make([]float64, 64)
	asymmetries := make([]float64, 64)

	for index := range radii {
		radii[index] = 0.1 + 0.01*float64(index)
		asymmetries[index] = 0.05 + 0.005*float64(index)
	}

	gates, ok := fitGatesFromHistory(radii, asymmetries)

	if !ok {
		testingT.Fatal("expected gate derivation to succeed with sufficient history")
	}

	if !gates.ready() {
		testingT.Fatalf("expected ready gates, got %+v", gates)
	}
}

func TestFitGateEstimatorMatchesFitGatesFromHistory(testingT *testing.T) {
	radii := make([]float64, 64)
	asymmetries := make([]float64, 64)

	for index := range radii {
		radii[index] = 0.1 + 0.01*float64(index)
		asymmetries[index] = 0.05 + 0.005*float64(index)
	}

	expected, ok := fitGatesFromHistory(radii, asymmetries)

	if !ok {
		testingT.Fatal("expected gate derivation to succeed")
	}

	estimator := newFitGateEstimator()
	actual, ok := estimator.measure(radii, asymmetries)

	if !ok {
		testingT.Fatal("expected estimator gate derivation to succeed")
	}

	if actual.saturationRadius != expected.saturationRadius {
		testingT.Fatalf(
			"saturationRadius mismatch: estimator=%v history=%v",
			actual.saturationRadius, expected.saturationRadius,
		)
	}

	if actual.frenzyAsymmetry != expected.frenzyAsymmetry {
		testingT.Fatalf(
			"frenzyAsymmetry mismatch: estimator=%v history=%v",
			actual.frenzyAsymmetry, expected.frenzyAsymmetry,
		)
	}
}
