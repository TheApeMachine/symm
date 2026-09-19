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
func NewDistributionNormalize(params ...types.Value[any, WeightsInput]) DistributionNormalize {
	return func(input WeightsInput) NormalizedReading {
		in := input
		if len(params) > 0 && params[0] != nil {
			in = params[0](input)
		}
		weights, total := normalizeWeights(in.Weights)
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
func NewWasserstein1(params ...types.Value[any, DistanceInput]) Wasserstein1 {
	return func(input DistanceInput) float64 {
		in := input
		if len(params) > 0 && params[0] != nil {
			in = params[0](input)
		}
		if len(in.Positions) == 0 || len(in.Positions) != len(in.WeightsA) || len(in.Positions) != len(in.WeightsB) {
			return math.Inf(1)
		}

		return wassersteinDistance(in.Positions, in.WeightsA, in.WeightsB)
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
func NewKolmogorovSmirnov(params ...types.Value[any, DistanceInput]) KolmogorovSmirnov {
	return func(input DistanceInput) float64 {
		in := input
		if len(params) > 0 && params[0] != nil {
			in = params[0](input)
		}
		if len(in.Positions) == 0 || len(in.Positions) != len(in.WeightsA) || len(in.Positions) != len(in.WeightsB) {
			return math.Inf(1)
		}

		return cumulativeDistance(in.Positions, in.WeightsA, in.WeightsB)
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
func NewDistributionEntropy(params ...types.Value[any, ShapeInput]) DistributionEntropy {
	return func(input ShapeInput) float64 {
		in := input
		if len(params) > 0 && params[0] != nil {
			in = params[0](input)
		}
		entropy := 0.0
		for _, weight := range in.Weights {
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
func NewConcentration(params ...types.Value[any, ShapeInput]) Concentration {
	return func(input ShapeInput) float64 {
		in := input
		if len(params) > 0 && params[0] != nil {
			in = params[0](input)
		}
		total := 0.0
		for _, weight := range in.Weights {
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
func NewSortedPositions(params ...types.Value[any, SortedInput]) SortedPositions {
	return func(input SortedInput) SortedReading {
		in := input
		if len(params) > 0 && params[0] != nil {
			in = params[0](input)
		}
		if len(in.Positions) != len(in.Weights) {
			return SortedReading{}
		}

		positions, weights := sortPositions(in.Positions, in.Weights)
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
func NewWasserstein1Pairs(params ...types.Value[any, PairsInput]) Wasserstein1Pairs {
	return func(input PairsInput) float64 {
		in := input
		if len(params) > 0 && params[0] != nil {
			in = params[0](input)
		}
		_, distance, _ := mergedWalk(in.Left, in.Right)
		return distance
	}
}

type KolmogorovSmirnovPairs types.Value[PairsInput, float64]
/*
NewKolmogorovSmirnovPairs creates a Value closure computing the Kolmogorov-Smirnov statistic
between two distributions given as ascending-sorted WeightedPoint streams via a merged walk.
No structs, pure Value closure.
*/
func NewKolmogorovSmirnovPairs(params ...types.Value[any, PairsInput]) KolmogorovSmirnovPairs {
	return func(input PairsInput) float64 {
		in := input
		if len(params) > 0 && params[0] != nil {
			in = params[0](input)
		}
		statistic, _, _ := mergedWalk(in.Left, in.Right)
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
	hasPrevious := false
	var previousPosition float64
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

		if hasPrevious {
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

		hasPrevious = true
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
func NewConcentrationPoints(params ...types.Value[any, PointsInput]) ConcentrationPoints {
	return func(input PointsInput) float64 {
		in := input
		if len(params) > 0 && params[0] != nil {
			in = params[0](input)
		}
		total := totalWeight(in.Points)
		concentration := 0.0

		if total != 0 {
			for _, point := range in.Points {
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
func NewEntropyPoints(params ...types.Value[any, PointsInput]) EntropyPoints {
	return func(input PointsInput) float64 {
		in := input
		if len(params) > 0 && params[0] != nil {
			in = params[0](input)
		}
		total := totalWeight(in.Points)
		entropy := 0.0

		if total != 0 {
			for _, point := range in.Points {
				if point.Weight > 0 {
					normalized := point.Weight / total
					entropy -= normalized * math.Log(normalized)
				}
			}
		}

		return entropy
	}
}
