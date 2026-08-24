package statistic

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolMahalanobisSNR      = types.MustIntern("mahalanobis/snr")
	SymbolMahalanobisDistance = types.MustIntern("mahalanobis/distance")
	SymbolMahalanobisReady    = types.MustIntern("mahalanobis/ready")
	SymbolMahalanobisSupport  = types.MustIntern("mahalanobis/support")
)

type mahalanobisSlots struct {
	snr      types.Symbol
	distance types.Symbol
	ready    types.Symbol
	support  types.Symbol
}

func newMahalanobisSlots(prefix string) mahalanobisSlots {
	return mahalanobisSlots{
		snr:      types.MustIntern(temporal.JoinPrefix(prefix, "mahalanobis/snr")),
		distance: types.MustIntern(temporal.JoinPrefix(prefix, "mahalanobis/distance")),
		ready:    types.MustIntern(temporal.JoinPrefix(prefix, "mahalanobis/ready")),
		support:  types.MustIntern(temporal.JoinPrefix(prefix, "mahalanobis/support")),
	}
}

/*
Mahalanobis evaluates the joint signal-to-noise ratio across k residual
dimensions against their causal empirical covariance matrix Sigma_{t-}:

	SNR_t = (1 / k) * delta_t^T * Sigma_{t-}^{-1} * delta_t

Evaluation is strictly causal: the current vector delta_t is evaluated against
the covariance of prior residuals before delta_t updates the covariance.
*/
func Mahalanobis(prefix string, residualSlots ...types.Symbol) types.Primitive {
	dimensionCount := len(residualSlots)
	slots := newMahalanobisSlots(prefix)

	covSlots := make([][]types.Symbol, dimensionCount)

	for row := range dimensionCount {
		covSlots[row] = make([]types.Symbol, dimensionCount)

		for column := range dimensionCount {
			slotName := fmt.Sprintf("mahalanobis/cov/%d/%d", row, column)
			covSlots[row][column] = types.MustIntern(temporal.JoinPrefix(prefix, slotName))
		}
	}

	return func(input types.Frame) types.Frame {
		if dimensionCount == 0 {
			input.Err = fmt.Errorf("statistic: mahalanobis requires at least one residual dimension")

			return input
		}

		residuals := make([]float64, dimensionCount)

		for index, slot := range residualSlots {
			value, found := input.Get(slot)

			if !found {
				input.Err = fmt.Errorf("statistic: mahalanobis missing residual slot %v", slot)

				return input
			}

			residuals[index] = value
		}

		currentSupport, _ := input.Get(slots.support)
		covariance := loadCovarianceMatrix(&input, covSlots, dimensionCount)

		if int(currentSupport) >= dimensionCount+1 {
			snr, distance, invertible := evaluateMahalanobisSNR(residuals, covariance, dimensionCount)

			if invertible {
				input.Put(slots.snr, snr)
				input.Put(slots.distance, distance)
				input.Put(slots.ready, 1)
			} else {
				input.Put(slots.snr, 0)
				input.Put(slots.distance, 0)
				input.Put(slots.ready, 0)
			}
		} else {
			input.Put(slots.snr, 0)
			input.Put(slots.distance, 0)
			input.Put(slots.ready, 0)
		}

		updateCausalCovariance(&input, covSlots, dimensionCount, residuals, currentSupport)
		input.Put(slots.support, currentSupport+1)

		return input
	}
}

func loadCovarianceMatrix(
	frame *types.Frame,
	slots [][]types.Symbol,
	dimensionCount int,
) []float64 {
	matrix := make([]float64, dimensionCount*dimensionCount)

	for row := range dimensionCount {
		for column := range dimensionCount {
			value, _ := frame.Get(slots[row][column])
			matrix[row*dimensionCount+column] = value
		}
	}

	return matrix
}

func updateCausalCovariance(
	frame *types.Frame,
	slots [][]types.Symbol,
	dimensionCount int,
	residuals []float64,
	support float64,
) {
	newSupport := support + 1.0

	for row := range dimensionCount {
		for column := range dimensionCount {
			oldValue, _ := frame.Get(slots[row][column])
			outerProduct := residuals[row] * residuals[column]
			updatedValue := (oldValue*support + outerProduct) / newSupport
			frame.Put(slots[row][column], updatedValue)
		}
	}
}

func evaluateMahalanobisSNR(
	residuals []float64,
	covariance []float64,
	dimensionCount int,
) (float64, float64, bool) {
	choleskyLower, decomposed := choleskyDecomposition(covariance, dimensionCount)

	if !decomposed {
		return 0, 0, false
	}

	// Solve L * y = residuals
	forwardSolve := make([]float64, dimensionCount)

	for row := range dimensionCount {
		currentSum := residuals[row]

		for column := range row {
			currentSum -= choleskyLower[row*dimensionCount+column] * forwardSolve[column]
		}

		diagonalElement := choleskyLower[row*dimensionCount+row]

		if diagonalElement <= 0 {
			return 0, 0, false
		}

		forwardSolve[row] = currentSum / diagonalElement
	}

	squaredDistance := 0.0

	for row := range dimensionCount {
		squaredDistance += forwardSolve[row] * forwardSolve[row]
	}

	if squaredDistance < 0 || math.IsNaN(squaredDistance) {
		return 0, 0, false
	}

	snr := squaredDistance / float64(dimensionCount)
	distance := math.Sqrt(squaredDistance)

	return snr, distance, true
}

func choleskyDecomposition(
	matrix []float64,
	dimensionCount int,
) ([]float64, bool) {
	lower := make([]float64, dimensionCount*dimensionCount)

	for row := range dimensionCount {
		for column := range row + 1 {
			sumOfProducts := 0.0

			for inner := range column {
				sumOfProducts += lower[row*dimensionCount+inner] * lower[column*dimensionCount+inner]
			}

			if row == column {
				diagonalDifference := matrix[row*dimensionCount+row] - sumOfProducts

				if diagonalDifference <= 1e-12 {
					return nil, false
				}

				lower[row*dimensionCount+column] = math.Sqrt(diagonalDifference)
				continue
			}

			diagonalDivisor := lower[column*dimensionCount+column]

			if diagonalDivisor <= 0 {
				return nil, false
			}

			lower[row*dimensionCount+column] = (matrix[row*dimensionCount+column] - sumOfProducts) / diagonalDivisor
		}
	}

	return lower, true
}
