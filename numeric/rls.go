package numeric

import (
	"fmt"
	"math"
)

/*
RLSFilter is a recursive least-squares linear predictor for online feature regression.
*/
type RLSFilter struct {
	dimension        int
	beta             []float64
	covariance       [][]float64
	forgettingFactor float64
}

/*
NewRLSFilter allocates an RLS filter with the given feature dimension (excluding intercept).
initialVariance sets the diagonal of the initial covariance matrix.
*/
func NewRLSFilter(dimension int, initialVariance float64) (*RLSFilter, error) {
	if dimension <= 0 {
		return nil, fmt.Errorf("numeric: rls dimension must be positive")
	}

	if initialVariance <= 0 {
		return nil, fmt.Errorf("numeric: rls initial variance must be positive")
	}

	size := dimension + 1
	covariance := make([][]float64, size)

	for row := 0; row < size; row++ {
		covariance[row] = make([]float64, size)
		covariance[row][row] = initialVariance
	}

	return &RLSFilter{
		dimension:        dimension,
		beta:             make([]float64, size),
		covariance:       covariance,
		forgettingFactor: 1,
	}, nil
}

/*
SetForgettingFactor sets lambda in (0, 1]. Values below one inflate covariance
before each update so older observations decay faster.
*/
func (filter *RLSFilter) SetForgettingFactor(lambda float64) error {
	if lambda <= 0 || lambda > 1 {
		return fmt.Errorf("numeric: rls forgetting factor must be in (0,1], got %g", lambda)
	}

	filter.forgettingFactor = lambda

	return nil
}

func (filter *RLSFilter) applyForgetting() error {
	if filter.forgettingFactor >= 1 {
		return nil
	}

	scale := 1 / filter.forgettingFactor

	for row := range filter.covariance {
		for col := range filter.covariance[row] {
			filter.covariance[row][col] *= scale
		}
	}

	return nil
}

/*
Observe ingests one feature vector and scalar target, updating coefficients in place.
*/
func (filter *RLSFilter) Observe(features []float64, target float64) error {
	if err := filter.applyForgetting(); err != nil {
		return err
	}

	if len(features) != filter.dimension {
		return fmt.Errorf(
			"numeric: rls expected %d features, got %d",
			filter.dimension,
			len(features),
		)
	}

	design := make([]float64, filter.dimension+1)
	design[0] = 1

	copy(design[1:], features)

	size := len(design)
	px := make([]float64, size)

	for row := 0; row < size; row++ {
		for col := 0; col < size; col++ {
			px[row] += filter.covariance[row][col] * design[col]
		}
	}

	denominator := 1.0

	for index := 0; index < size; index++ {
		denominator += design[index] * px[index]
	}

	if denominator <= 0 {
		return fmt.Errorf("numeric: rls denominator must be positive")
	}

	prediction := 0.0

	for index := 0; index < size; index++ {
		prediction += filter.beta[index] * design[index]
	}

	innovation := target - prediction

	for row := 0; row < size; row++ {
		gain := px[row] / denominator
		filter.beta[row] += gain * innovation

		for col := 0; col < size; col++ {
			filter.covariance[row][col] -= gain * px[col]
		}
	}

	return nil
}

/*
Predict returns the intercept plus feature-weighted forecast.
*/
func (filter *RLSFilter) Predict(features []float64) (float64, error) {
	if len(features) != filter.dimension {
		return 0, fmt.Errorf(
			"numeric: rls expected %d features, got %d",
			filter.dimension,
			len(features),
		)
	}

	forecast := filter.beta[0]

	for index, feature := range features {
		forecast += filter.beta[index+1] * feature
	}

	return forecast, nil
}

/*
Coefficients returns a copy of the current coefficient vector including intercept.
*/
func (filter *RLSFilter) Coefficients() []float64 {
	out := make([]float64, len(filter.beta))
	copy(out, filter.beta)

	return out
}

/*
SetCoefficients restores coefficients when replaying from a checkpoint.
*/
func (filter *RLSFilter) SetCoefficients(coefficients []float64) error {
	if len(coefficients) != len(filter.beta) {
		return fmt.Errorf(
			"numeric: rls expected %d coefficients, got %d",
			len(filter.beta),
			len(coefficients),
		)
	}

	copy(filter.beta, coefficients)

	return nil
}

/*
Residual returns target minus prediction for diagnostics.
*/
func (filter *RLSFilter) Residual(features []float64, target float64) (float64, error) {
	prediction, err := filter.Predict(features)

	if err != nil {
		return 0, err
	}

	return target - prediction, nil
}

/*
ConditionNumberBound estimates coefficient stability from covariance diagonal.
*/
func (filter *RLSFilter) ConditionNumberBound() float64 {
	minDiagonal := math.Inf(1)
	maxDiagonal := 0.0

	for row := range filter.covariance {
		value := filter.covariance[row][row]

		if value < minDiagonal {
			minDiagonal = value
		}

		if value > maxDiagonal {
			maxDiagonal = value
		}
	}

	if minDiagonal <= 0 {
		return math.Inf(1)
	}

	return maxDiagonal / minDiagonal
}
