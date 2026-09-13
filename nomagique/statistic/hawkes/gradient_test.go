package hawkes

import (
	"math"
	"testing"
)

func TestLogLikelihoodGradientMatchesFiniteDifference(testingT *testing.T) {
	buy := make([]float64, 0, 20)
	sell := make([]float64, 0, 20)

	for index := 0; index < 20; index++ {
		base := float64(index) * 0.6
		buy = append(buy, base)
		sell = append(sell, base+0.25)
	}

	stream := newArrivalStream(buy, sell)
	horizonSec := 13.0
	fit := bivariateFit{
		muX: 0.4, muY: 0.35,
		alphaXX: 0.12, alphaXY: 0.05, alphaYX: 0.05, alphaYY: 0.1,
		beta: 1.1,
	}

	logLikelihood, gradient, ok := fit.logLikelihoodGradient(stream, horizonSec)

	if !ok {
		testingT.Fatal("expected a valid gradient")
	}

	if math.IsInf(logLikelihood, 0) || math.IsNaN(logLikelihood) {
		testingT.Fatalf("expected a finite log-likelihood, got %v", logLikelihood)
	}

	const step = 1e-6
	natural := [bivariateParamCount]func(delta float64) bivariateFit{
		func(delta float64) bivariateFit { perturbed := fit; perturbed.muX += delta; return perturbed },
		func(delta float64) bivariateFit { perturbed := fit; perturbed.muY += delta; return perturbed },
		func(delta float64) bivariateFit { perturbed := fit; perturbed.alphaXX += delta; return perturbed },
		func(delta float64) bivariateFit { perturbed := fit; perturbed.alphaXY += delta; return perturbed },
		func(delta float64) bivariateFit { perturbed := fit; perturbed.alphaYX += delta; return perturbed },
		func(delta float64) bivariateFit { perturbed := fit; perturbed.alphaYY += delta; return perturbed },
		func(delta float64) bivariateFit { perturbed := fit; perturbed.beta += delta; return perturbed },
	}

	for index, perturb := range natural {
		up := perturb(step).logLikelihood(stream, horizonSec)
		down := perturb(-step).logLikelihood(stream, horizonSec)
		numeric := (up - down) / (2 * step)

		if math.Abs(gradient[index]-numeric) > 1e-3*math.Max(1, math.Abs(numeric)) {
			testingT.Fatalf(
				"gradient[%d] = %v, finite-difference = %v",
				index, gradient[index], numeric,
			)
		}
	}
}

func TestLogSpaceGradientChainRule(testingT *testing.T) {
	fit := bivariateFit{
		muX: 0.5, muY: 0.4,
		alphaXX: 0.2, alphaXY: 0.1, alphaYX: 0.1, alphaYY: 0.2,
		beta: 1.0,
	}
	natural := [bivariateParamCount]float64{1, 1, 1, 1, 1, 1, 1}
	logSpace := logSpaceGradient(natural, fit)

	if math.Abs(logSpace[0]-fit.muX) > 1e-9 {
		testingT.Fatalf("expected d/d(logMuX) = muX, got %v", logSpace[0])
	}

	if math.Abs(logSpace[1]-fit.muY) > 1e-9 {
		testingT.Fatalf("expected d/d(logMuY) = muY, got %v", logSpace[1])
	}
}
