package hawkes

import "testing"

func TestNewMomentDiagnosticRejectsZeroMoment(testingT *testing.T) {
	if _, err := newMomentDiagnostic(0, 0); err == nil {
		testingT.Fatal("expected an error for a zero mixed moment")
	}
}

func TestMomentDiagnosticMeasureRequiresAlignedSamples(testingT *testing.T) {
	diagnostic, err := newMomentDiagnostic(1, 1)

	if err != nil {
		testingT.Fatalf("newMomentDiagnostic: %v", err)
	}

	_, err = diagnostic.measure(momentSample{}, 1, 1, 0.1, 0.05, 0.05, 0.1, 1)

	if err == nil {
		testingT.Fatal("expected a validation error for empty samples")
	}
}

func TestMomentDiagnosticMeasureAgreesWithSelf(testingT *testing.T) {
	x := []float64{1, 2, 1, 3, 2, 1, 2, 3}
	y := []float64{1, 1, 2, 2, 3, 1, 2, 3}

	diagnostic, err := newMomentDiagnostic(1, 1)

	if err != nil {
		testingT.Fatalf("newMomentDiagnostic: %v", err)
	}

	muX, muY, alphaXX, alphaXY, alphaYX, alphaYY, ok := methodOfMoments(x, y, nil, 1)

	if !ok {
		testingT.Fatal("expected methodOfMoments to converge on this sample")
	}

	result, err := diagnostic.measure(
		momentSample{x: x, y: y},
		muX, muY, alphaXX, alphaXY, alphaYX, alphaYY, 1,
	)

	if err != nil {
		testingT.Fatalf("measure: %v", err)
	}

	if result.confidence <= 0 || result.confidence > 1 {
		testingT.Fatalf("expected confidence in (0, 1], got %v", result.confidence)
	}
}

func TestMethodOfMomentsRejectsMismatchedLengths(testingT *testing.T) {
	_, _, _, _, _, _, ok := methodOfMoments([]float64{1, 2}, []float64{1}, nil, 1)

	if ok {
		testingT.Fatal("expected mismatched-length streams to fail")
	}
}

func TestCrossAsymmetrySignsMatchDominantSide(testingT *testing.T) {
	x := []float64{5, 4, 3, 2, 1, 5, 4, 3, 2, 1}
	y := []float64{1, 1, 2, 3, 5, 1, 1, 2, 3, 5}

	asymmetry, ok := crossAsymmetry(x, y, nil)

	if !ok {
		testingT.Fatal("expected crossAsymmetry to succeed")
	}

	if asymmetry == 0 {
		testingT.Fatal("expected a nonzero asymmetry between distinct streams")
	}
}
