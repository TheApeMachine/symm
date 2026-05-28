package causal

import (
	"math"
	"testing"
)

func TestRidgeSolveHandlesCollinearPredictors(t *testing.T) {
	size := minCausalHistory
	target := make([]float64, size)
	first := make([]float64, size)
	second := make([]float64, size)
	third := make([]float64, size)

	for index := 0; index < size; index++ {
		first[index] = 1
		second[index] = 2
		third[index] = float64(index) * 0.05
		target[index] = third[index]*0.4 + 0.01
	}

	_, plainOK := ols3Plain(target, first, second, third)
	coef, ridgeOK := ols3(target, first, second, third)

	if plainOK {
		t.Fatal("expected plain OLS to fail on collinear predictors")
	}

	if !ridgeOK {
		t.Fatal("expected ridge OLS to succeed on collinear predictors")
	}

	if coef[3] <= 0 {
		t.Fatalf("expected positive flow coefficient, got %v", coef[3])
	}
}

func TestConditionEstimateDetectsIllConditionedMatrix(t *testing.T) {
	normal := [][]float64{
		{120, 118, 116},
		{0.001, 0.001, 0.001},
		{116, 114, 112},
	}

	if conditionEstimate(normal) <= minConditionRatio {
		t.Fatal("expected high condition estimate for near-singular matrix")
	}
}

func TestConditionEstimateDetectsCollinearity(t *testing.T) {
	normal := [][]float64{
		{1, 1},
		{1, 1.00001},
	}

	if conditionEstimate(normal) <= minConditionRatio {
		t.Fatal("expected high condition estimate for collinear matrix")
	}

	if ridgeLambda(normal) <= 0 {
		t.Fatal("expected positive ridge lambda")
	}
}

func TestConditionEstimateRejectsRaggedMatrix(t *testing.T) {
	normal := [][]float64{
		{1, 0},
		{0},
	}

	if !math.IsInf(conditionEstimate(normal), 1) {
		t.Fatal("expected infinite condition estimate for ragged matrix")
	}
}

func TestConditionEstimateIgnoresScaleDisparity(t *testing.T) {
	normal := [][]float64{
		{1e-4, 0, 0},
		{0, 1e8, 0},
		{0, 0, 1},
	}

	if condition := conditionEstimate(normal); condition != 1 {
		t.Fatalf("expected unit condition estimate, got %v", condition)
	}
}

func ols3Plain(
	target, first, second, third []float64,
) ([]float64, bool) {
	size := len(target)
	normal := make([][]float64, 4)

	for row := 0; row < 4; row++ {
		normal[row] = make([]float64, 4)
	}

	targetVec := make([]float64, 4)

	for index := 0; index < size; index++ {
		predictors := []float64{1, first[index], second[index], third[index]}

		for row := 0; row < 4; row++ {
			targetVec[row] += predictors[row] * target[index]

			for col := 0; col < 4; col++ {
				normal[row][col] += predictors[row] * predictors[col]
			}
		}
	}

	return gaussianSolve(normal, targetVec)
}

func BenchmarkRidgeSolve(b *testing.B) {
	normal := [][]float64{
		{12, 2, 3, 4},
		{2, 9, 1, 2},
		{3, 1, 11, 1},
		{4, 2, 1, 10},
	}
	vector := []float64{1, 2, 3, 4}

	b.ReportAllocs()

	for b.Loop() {
		if _, ok := ridgeSolve(normal, vector); !ok {
			b.Fatal("expected ridge solve")
		}
	}
}
