package hawkes

import (
	"math"
	"testing"
)

func TestFitStageMeasureRecoversExcitation(testingT *testing.T) {
	buy := make([]float64, 0, 40)
	sell := make([]float64, 0, 40)

	for index := 0; index < 40; index++ {
		base := float64(index) * 0.5
		buy = append(buy, base)
		sell = append(sell, base+0.2)
	}

	stage, err := newFitStage(fitStageConfig{horizonSec: 25})

	if err != nil {
		testingT.Fatalf("newFitStage: %v", err)
	}

	output, err := stage.measure(fitStageInput{xTimesSec: buy, yTimesSec: sell})

	if err != nil {
		testingT.Fatalf("measure: %v", err)
	}

	if !output.fit.valid() {
		testingT.Fatalf("expected a valid fit, got %+v", output.fit)
	}

	if output.spectralRadius < 0 || output.spectralRadius >= criticalBranch {
		testingT.Fatalf("spectral radius out of range: %v", output.spectralRadius)
	}

	if output.value <= 0 {
		testingT.Fatalf("expected positive excitation evidence, got %v", output.value)
	}
}

func TestFitStageRequiresHorizon(testingT *testing.T) {
	if _, err := newFitStage(fitStageConfig{}); err == nil {
		testingT.Fatal("expected an error when horizon is zero")
	}
}

func TestFitStageMeasureRequiresAlignedTimestamps(testingT *testing.T) {
	stage, err := newFitStage(fitStageConfig{horizonSec: 10})

	if err != nil {
		testingT.Fatalf("newFitStage: %v", err)
	}

	if _, err := stage.measure(fitStageInput{}); err == nil {
		testingT.Fatal("expected a validation error for empty input")
	}
}

func TestFitStageMeasureRejectsSparseData(testingT *testing.T) {
	stage, err := newFitStage(fitStageConfig{horizonSec: 10})

	if err != nil {
		testingT.Fatalf("newFitStage: %v", err)
	}

	if _, err := stage.measure(fitStageInput{xTimesSec: []float64{1}}); err == nil {
		testingT.Fatal("expected a validation error for a single event")
	}
}

func TestBivariateEstimatorSelfOnlyZeroesCrossTerms(testingT *testing.T) {
	buy := make([]float64, 0, 40)
	sell := make([]float64, 0, 40)

	for index := 0; index < 40; index++ {
		base := float64(index) * 0.5
		buy = append(buy, base)
		sell = append(sell, base+0.2)
	}

	stream := newArrivalStream(buy, sell)
	fit := newBivariateEstimator(bivariateFit{}).fitSelfOnly(stream, 25)

	if !fit.valid() {
		testingT.Fatalf("expected a valid self-only fit, got %+v", fit)
	}

	if fit.alphaXY != 0 || fit.alphaYX != 0 {
		testingT.Fatalf("expected zeroed cross terms, got alphaXY=%v alphaYX=%v", fit.alphaXY, fit.alphaYX)
	}
}

func TestLogLikelihoodToleranceScalesWithMagnitude(testingT *testing.T) {
	small := logLikelihoodTolerance(1, 1)
	large := logLikelihoodTolerance(1e6, 1e6)

	if !(large > small) {
		testingT.Fatalf("expected tolerance to scale with magnitude: small=%v large=%v", small, large)
	}

	if math.IsNaN(small) || math.IsInf(small, 0) {
		testingT.Fatalf("expected a finite tolerance, got %v", small)
	}
}
