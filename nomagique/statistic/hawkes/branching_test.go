package hawkes

import (
	"math"
	"testing"
)

func TestSpectralRadiusDiagonalMatrix(testingT *testing.T) {
	matrix := [2][2]float64{{0.3, 0}, {0, 0.6}}

	if got := spectralRadius(matrix); math.Abs(got-0.6) > 1e-9 {
		testingT.Fatalf("expected spectral radius 0.6, got %v", got)
	}
}

func TestSpectralRadiusComplexEigenvalues(testingT *testing.T) {
	matrix := [2][2]float64{{0, 0.5}, {-0.5, 0}}
	got := spectralRadius(matrix)

	if math.Abs(got-0.5) > 1e-9 {
		testingT.Fatalf("expected modulus 0.5 for complex eigenvalues, got %v", got)
	}
}

func TestMeanIntensityMatchesClosedForm(testingT *testing.T) {
	lambdaX, lambdaY, ok := meanIntensity(1, 1, 0.2, 0.1, 0.1, 0.2, 1)

	if !ok {
		testingT.Fatal("expected meanIntensity to succeed for a subcritical process")
	}

	if lambdaX <= 0 || lambdaY <= 0 {
		testingT.Fatalf("expected positive mean intensities, got lambdaX=%v lambdaY=%v", lambdaX, lambdaY)
	}
}

func TestMeanIntensityRejectsNonPositiveBeta(testingT *testing.T) {
	if _, _, ok := meanIntensity(1, 1, 0.2, 0.1, 0.1, 0.2, 0); ok {
		testingT.Fatal("expected meanIntensity to fail for beta <= 0")
	}
}

func TestImmediateOffspringSumsColumns(testingT *testing.T) {
	buyParent, sellParent, ok := immediateOffspring(0.2, 0.1, 0.1, 0.2, 1)

	if !ok {
		testingT.Fatal("expected immediateOffspring to succeed")
	}

	if math.Abs(buyParent-0.3) > 1e-9 || math.Abs(sellParent-0.3) > 1e-9 {
		testingT.Fatalf("expected buyParent=sellParent=0.3, got %v %v", buyParent, sellParent)
	}
}

func TestTotalDescendantsExceedsImmediateOffspring(testingT *testing.T) {
	immediateBuy, immediateSell, ok := immediateOffspring(0.3, 0.1, 0.1, 0.3, 1)

	if !ok {
		testingT.Fatal("expected immediateOffspring to succeed")
	}

	totalBuy, totalSell, ok := totalDescendants(0.3, 0.1, 0.1, 0.3, 1)

	if !ok {
		testingT.Fatal("expected totalDescendants to succeed")
	}

	if totalBuy < immediateBuy || totalSell < immediateSell {
		testingT.Fatalf(
			"expected total descendants to be at least immediate offspring: total=(%v,%v) immediate=(%v,%v)",
			totalBuy, totalSell, immediateBuy, immediateSell,
		)
	}
}

func TestTotalDescendantsRejectsSupercriticalProcess(testingT *testing.T) {
	if _, _, ok := totalDescendants(1.0, 0.1, 0.1, 1.0, 1); ok {
		testingT.Fatal("expected totalDescendants to fail for a supercritical process")
	}
}
