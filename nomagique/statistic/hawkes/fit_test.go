package hawkes

import (
	"math"
	"testing"
)

func TestLogLikelihoodRejectsNonPositiveSpan(testingT *testing.T) {
	fit := bivariateFit{
		muX:     0.5,
		muY:     0.5,
		alphaXX: 0.1,
		alphaXY: 0.05,
		alphaYX: 0.05,
		alphaYY: 0.1,
		beta:    1.0,
	}

	stream := newArrivalStream([]float64{10.0}, []float64{10.0})

	if _, ok := fit.logLikelihood(stream, 10.0); ok {
		testingT.Fatal("expected logLikelihood to fail when horizon equals origin (zero span)")
	}

	if _, ok := fit.logLikelihood(stream, 5.0); ok {
		testingT.Fatal("expected logLikelihood to fail when horizon is before origin (negative span)")
	}
}

func TestLogLikelihoodRejectsInvalidFitParameters(testingT *testing.T) {
	stream := newArrivalStream([]float64{10.0}, []float64{12.0})

	nonPositiveMu := bivariateFit{
		muX:  0.0,
		muY:  0.5,
		beta: 1.0,
	}

	if _, ok := nonPositiveMu.logLikelihood(stream, 15.0); ok {
		testingT.Fatal("expected logLikelihood to fail for muX <= 0")
	}

	negativeAlpha := bivariateFit{
		muX:     0.5,
		muY:     0.5,
		alphaXX: -0.1,
		beta:    1.0,
	}

	if _, ok := negativeAlpha.logLikelihood(stream, 15.0); ok {
		testingT.Fatal("expected logLikelihood to fail for alphaXX < 0")
	}
}

func TestLogLikelihoodSucceedsOnValidStream(testingT *testing.T) {
	fit := bivariateFit{
		muX:     0.4,
		muY:     0.35,
		alphaXX: 0.12,
		alphaXY: 0.05,
		alphaYX: 0.05,
		alphaYY: 0.1,
		beta:    1.1,
	}

	stream := newArrivalStream(
		[]float64{1.0, 2.5, 3.2, 5.0},
		[]float64{1.5, 2.8, 4.0, 5.5},
	)

	ll, ok := fit.logLikelihood(stream, 6.0)

	if !ok {
		testingT.Fatal("expected logLikelihood to succeed on valid stream")
	}

	if math.IsNaN(ll) || math.IsInf(ll, 0) {
		testingT.Fatalf("expected finite log-likelihood, got %v", ll)
	}
}
