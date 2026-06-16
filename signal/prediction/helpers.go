package prediction

import (
	"fmt"
	"math"

	"github.com/spf13/viper"
	"github.com/theapemachine/nomagique/probability"
)

func measurementsCapacity() int {
	capacity := viper.GetInt("signals.prediction.measurements_capacity")

	if capacity <= 0 {
		capacity = viper.GetInt("signals.measurements_capacity")
	}

	if capacity <= 0 {
		return 64
	}

	return capacity
}

func boundedClassifierAlpha() float64 {
	return 0.1
}

func forgettingFactor() float64 {
	return 1 - 0.1
}

func resolvedChange(prices []float64) (move float64, magnitude float64, ok bool) {
	if len(prices) < 2 {
		return 0, 0, false
	}

	first := prices[0]
	last := prices[len(prices)-1]

	if first <= 0 || last <= 0 {
		return 0, 0, false
	}

	move = (last - first) / first
	magnitude = math.Abs(move)

	return move, magnitude, true
}

func touchSpread(prices []float64) (float64, error) {
	if len(prices) < 2 {
		return 0, fmt.Errorf("prediction: need at least two prices")
	}

	minPrice := prices[0]
	maxPrice := prices[0]

	for _, price := range prices[1:] {
		if price < minPrice {
			minPrice = price
		}

		if price > maxPrice {
			maxPrice = price
		}
	}

	spread := maxPrice - minPrice

	if spread <= 0 {
		return 0, fmt.Errorf("prediction: non-positive spread")
	}

	return spread, nil
}

func anchorChange(anchor, price float64) (fractional float64, magnitude float64) {
	if anchor <= 0 {
		return 0, 0
	}

	fractional = (price - anchor) / anchor
	magnitude = math.Abs(fractional)

	return fractional, magnitude
}

func movementUnits(value, scale float64) (float64, error) {
	if scale <= 0 || value == 0 {
		return 0, nil
	}

	sign := 1.0

	if value < 0 {
		sign = -1
	}

	forwardScore := math.Abs(value) / scale
	probabilities, err := probability.SoftmaxScoresNormalized([]float64{forwardScore, 1.0})

	if err != nil {
		return 0, err
	}

	return sign * probabilities[0], nil
}

func sumFloats(values []float64) float64 {
	total := 0.0

	for _, value := range values {
		total += value
	}

	return total
}
