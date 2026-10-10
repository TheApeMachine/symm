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

func TestTotalDescendants(testingT *testing.T) {
	totalBuy, totalSell, ok := totalDescendants(0.3, 0.1, 0.1, 0.3, 1)

	if !ok {
		testingT.Fatal("expected totalDescendants to succeed")
	}

	if totalBuy <= 0 || totalSell <= 0 {
		testingT.Fatalf("expected positive total descendants, got totalBuy=%v totalSell=%v", totalBuy, totalSell)
	}
}

func TestTotalDescendantsRejectsSupercriticalProcess(testingT *testing.T) {
	if _, _, ok := totalDescendants(1.0, 0.1, 0.1, 1.0, 1); ok {
		testingT.Fatal("expected totalDescendants to fail for a supercritical process")
	}
}
