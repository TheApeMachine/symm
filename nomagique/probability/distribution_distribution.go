/*
Package probability provides distance and shape Value closures for
univariate distributions expressed as a weight at each of a sorted real
coordinate. Callers supply weights and positions and this package answers
"how far apart are two shapes" and "how concentrated is one shape".
*/
package probability

import (
	"math"
	"sort"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
WeightsInput carries one weight vector to normalize.
*/
type WeightsInput struct {
	Weights []float64
}

/*
NormalizedReading is the unit-sum weight vector and the total it was scaled by.
*/
type NormalizedReading struct {
	Weights []float64
	Total   float64
}

type DistributionNormalize types.Value[WeightsInput, NormalizedReading]
/*
NewDistributionNormalize creates a Value closure scaling non-negative weights to a unit sum.
Negative weights are treated as zero.
No structs, pure Value closure.
*/
func NewDistributionNormalize() DistributionNormalize {
	return func(input WeightsInput) NormalizedReading {
		weights, total := normalizeWeights(input.Weights)
		return NormalizedReading{Weights: weights, Total: total}
	}
}

func normalizeWeights(weights []float64) ([]float64, float64) {
	normalized := make([]float64, len(weights))
	total := 0.0

	for _, weight := range weights {
		if weight > 0 {
			total += weight
		}
	}

	if total == 0 {
		return normalized, 0
	}

	for index, weight := range weights {
		if weight > 0 {
			normalized[index] = weight / total
		}
	}

	return normalized, total
}

/*
DistanceInput carries two weight vectors over one shared sorted position support.
*/
type DistanceInput struct {
	Positions []float64
	WeightsA  []float64
	WeightsB  []float64
}

type Wasserstein1 types.Value[DistanceInput, float64]
/*
NewWasserstein1 creates a Value closure computing the first Wasserstein distance
between two distributions over the same sorted position support.
No structs, pure Value closure.
*/
func NewWasserstein1() Wasserstein1 {
	return func(input DistanceInput) float64 {
		if len(input.Positions) == 0 || len(input.Positions) != len(input.WeightsA) || len(input.Positions) != len(input.WeightsB) {
			return math.Inf(1)
		}

		return wassersteinDistance(input.Positions, input.WeightsA, input.WeightsB)
	}
}

func wassersteinDistance(positions, weightsA, weightsB []float64) float64 {
	normalizedA, totalA := normalizeWeights(weightsA)
	normalizedB, totalB := normalizeWeights(weightsB)

	if totalA == 0 || totalB == 0 {
		return math.Inf(1)
	}

	cumulative := 0.0
	distance := 0.0

	for index := 0; index < len(positions)-1; index++ {
		cumulative += normalizedA[index] - normalizedB[index]
		width := positions[index+1] - positions[index]

		if width > 0 {
			distance += math.Abs(cumulative) * width
		}
	}

	return distance
}

type KolmogorovSmirnov types.Value[DistanceInput, float64]
/*
NewKolmogorovSmirnov creates a Value closure computing the Kolmogorov-Smirnov statistic
between two distributions over the same sorted position support.
No structs, pure Value closure.
*/
func NewKolmogorovSmirnov() KolmogorovSmirnov {
	return func(input DistanceInput) float64 {
		if len(input.Positions) == 0 || len(input.Positions) != len(input.WeightsA) || len(input.Positions) != len(input.WeightsB) {
			return math.Inf(1)
		}

		return cumulativeDistance(input.Positions, input.WeightsA, input.WeightsB)
	}
}

func cumulativeDistance(positions, weightsA, weightsB []float64) float64 {
	normalizedA, totalA := normalizeWeights(weightsA)
	normalizedB, totalB := normalizeWeights(weightsB)

	if totalA == 0 || totalB == 0 {
		return math.Inf(1)
	}

	cumulativeA := 0.0
	cumulativeB := 0.0
	statistic := 0.0

	for index := 0; index < len(positions); index++ {
		cumulativeA += normalizedA[index]
		cumulativeB += normalizedB[index]

		difference := math.Abs(cumulativeA - cumulativeB)
		if difference > statistic {
			statistic = difference
		}
	}

	return statistic
}

/*
ShapeInput carries one already-normalized weight vector.
*/
type ShapeInput struct {
	Weights []float64
}

type DistributionEntropy types.Value[ShapeInput, float64]
/*
NewDistributionEntropy creates a Value closure computing the Shannon entropy (nats) of a distribution.
No structs, pure Value closure.
*/
func NewDistributionEntropy() DistributionEntropy {
	return func(input ShapeInput) float64 {
		entropy := 0.0
		for _, weight := range input.Weights {
			if weight > 0 {
				entropy -= weight * math.Log(weight)
			}
		}

		return entropy
	}
}

type Concentration types.Value[ShapeInput, float64]
/*
NewConcentration creates a Value closure computing the Herfindahl index of a distribution: sum of squared weights.
No structs, pure Value closure.
*/
func NewConcentration() Concentration {
	return func(input ShapeInput) float64 {
		total := 0.0
		for _, weight := range input.Weights {
			total += weight * weight
		}

		return total
	}
}

/*
SortedInput carries unsorted positions with their weights.
*/
type SortedInput struct {
	Positions []float64
	Weights   []float64
}

/*
SortedReading is the ascending-sorted position clone with its weights reordered to match.
*/
type SortedReading struct {
	Positions []float64
	Weights   []float64
}

type SortedPositions types.Value[SortedInput, SortedReading]
/*
NewSortedPositions creates a Value closure that sorts unsorted positions ascending and reorders weights to match.
No structs, pure Value closure.
*/
func NewSortedPositions() SortedPositions {
	return func(input SortedInput) SortedReading {
		if len(input.Positions) != len(input.Weights) {
			return SortedReading{}
		}

		positions, weights := sortPositions(input.Positions, input.Weights)
		return SortedReading{Positions: positions, Weights: weights}
	}
}

func sortPositions(positions []float64, weights []float64) ([]float64, []float64) {
	indexed := make([]positionWeight, len(positions))
	for index, position := range positions {
		indexed[index] = positionWeight{position: position, weight: weights[index]}
	}

	sort.Slice(indexed, func(left, right int) bool {
		return indexed[left].position < indexed[right].position
	})

	sortedPositions := make([]float64, len(indexed))
	sortedWeights := make([]float64, len(indexed))

	for index, item := range indexed {
		sortedPositions[index] = item.position
		sortedWeights[index] = item.weight
	}

	return sortedPositions, sortedWeights
}

type positionWeight struct {
	position float64
	weight   float64
}

/*
WeightedPoint is one (position, weight) observation of a distribution.
*/
type WeightedPoint struct {
	Position float64
	Weight   float64
}

/*
PairsInput carries two ascending-sorted point streams.
*/
type PairsInput struct {
	Left  []WeightedPoint
	Right []WeightedPoint
}

type Wasserstein1Pairs types.Value[PairsInput, float64]
/*
NewWasserstein1Pairs creates a Value closure computing the first Wasserstein distance
between two distributions given as ascending-sorted WeightedPoint streams via a merged walk.
No structs, pure Value closure.
*/
func NewWasserstein1Pairs() Wasserstein1Pairs {
	return func(input PairsInput) float64 {
		_, distance, _ := mergedWalk(input.Left, input.Right)
		return distance
	}
}

type KolmogorovSmirnovPairs types.Value[PairsInput, float64]
/*
NewKolmogorovSmirnovPairs creates a Value closure computing the Kolmogorov-Smirnov statistic
between two distributions given as ascending-sorted WeightedPoint streams via a merged walk.
No structs, pure Value closure.
*/
func NewKolmogorovSmirnovPairs() KolmogorovSmirnovPairs {
	return func(input PairsInput) float64 {
		statistic, _, _ := mergedWalk(input.Left, input.Right)
		return statistic
	}
}

func mergedWalk(left []WeightedPoint, right []WeightedPoint) (float64, float64, int) {
	leftTotal := totalWeight(left)
	rightTotal := totalWeight(right)

	if leftTotal == 0 || rightTotal == 0 {
		return math.Inf(1), math.Inf(1), 0
	}

	leftIndex := 0
	rightIndex := 0
	cumulativeLeft := 0.0
	cumulativeRight := 0.0
	previousPosition := math.NaN()
	statistic := 0.0
	distance := 0.0
	distinct := 0

	for leftIndex < len(left) || rightIndex < len(right) {
		var position float64
		advanceLeft := false
		advanceRight := false

		switch {
		case leftIndex >= len(left):
			position = right[rightIndex].Position
			advanceRight = true
		case rightIndex >= len(right):
			position = left[leftIndex].Position
			advanceLeft = true
		default:
			leftPosition := left[leftIndex].Position
			rightPosition := right[rightIndex].Position

			switch {
			case leftPosition < rightPosition:
				position = leftPosition
				advanceLeft = true
			case rightPosition < leftPosition:
				position = rightPosition
				advanceRight = true
			default:
				position = leftPosition
				advanceLeft = true
				advanceRight = true
			}
		}

		if !math.IsNaN(previousPosition) {
			width := position - previousPosition
			if width > 0 {
				distance += math.Abs(cumulativeLeft-cumulativeRight) * width
			}
		}

		if advanceLeft {
			for leftIndex < len(left) && left[leftIndex].Position == position {
				cumulativeLeft += left[leftIndex].Weight / leftTotal
				leftIndex++
			}
		}

		if advanceRight {
			for rightIndex < len(right) && right[rightIndex].Position == position {
				cumulativeRight += right[rightIndex].Weight / rightTotal
				rightIndex++
			}
		}

		difference := math.Abs(cumulativeLeft - cumulativeRight)
		if difference > statistic {
			statistic = difference
		}

		previousPosition = position
		distinct++
	}

	return statistic, distance, distinct
}

func totalWeight(points []WeightedPoint) float64 {
	total := 0.0
	for _, point := range points {
		if point.Weight > 0 {
			total += point.Weight
		}
	}
	return total
}

/*
PointsInput carries one point stream.
*/
type PointsInput struct {
	Points []WeightedPoint
}

type ConcentrationPoints types.Value[PointsInput, float64]
/*
NewConcentrationPoints creates a Value closure computing Herfindahl concentration of a point stream.
No structs, pure Value closure.
*/
func NewConcentrationPoints() ConcentrationPoints {
	return func(input PointsInput) float64 {
		total := totalWeight(input.Points)
		concentration := 0.0

		if total != 0 {
			for _, point := range input.Points {
				if point.Weight > 0 {
					normalized := point.Weight / total
					concentration += normalized * normalized
				}
			}
		}

		return concentration
	}
}

type EntropyPoints types.Value[PointsInput, float64]
/*
NewEntropyPoints creates a Value closure computing Shannon entropy (nats) of a point stream.
No structs, pure Value closure.
*/
func NewEntropyPoints() EntropyPoints {
	return func(input PointsInput) float64 {
		total := totalWeight(input.Points)
		entropy := 0.0

		if total != 0 {
			for _, point := range input.Points {
				if point.Weight > 0 {
					normalized := point.Weight / total
					entropy -= normalized * math.Log(normalized)
				}
			}
		}

		return entropy
	}
}
