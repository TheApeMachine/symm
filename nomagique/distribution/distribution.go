/*
Package distribution provides pure, generic distance and shape mathematics for
univariate distributions expressed as a weight at each of a sorted real
coordinate. It is deliberately free of market semantics: callers supply
weights (quantities, notions, probabilities) and positions (prices, spreads,
levels) and this package only answers "how far apart are two shapes" and "how
concentrated is one shape".

The canonical inputs are a position slice and a matching weight slice of equal
length, positions sorted ascending. Weights are non-negative and normalized
internally, so a distribution is always a probability mass over its positions.
Every function is stateless and allocation-free in the hot path (a couple of
distance/statistic functions allocate a small working slice); nothing here
retains state or is causal beyond its inputs.
*/
package distribution

import (
	"math"
	"sort"
)

/*
Normalize scales non-negative weights to a unit sum and returns the normalized
weights and the total. Negative weights are treated as zero. A zero total
returns an all-zero slice and total 0; callers must treat a zero-total
distribution as empty rather than divide through it.
*/
func Normalize(weights []float64) ([]float64, float64) {
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
Wasserstein1 returns the first Wasserstein (earth mover's) distance between two
distributions over the same sorted position support: the integral of the
absolute difference of their cumulative masses, the minimal total transport
mass times distance to morph one shape into the other. positions must be
sorted ascending and have the same length/order as weightsA and weightsB;
weights are normalized internally. Value has the same units as positions
(before any caller normalization), and is 0 for identical shapes.
*/
func Wasserstein1(positions, weightsA, weightsB []float64) float64 {
	if len(positions) == 0 || len(positions) != len(weightsA) || len(positions) != len(weightsB) {
		return math.Inf(1)
	}

	normalizedA, totalA := Normalize(weightsA)
	normalizedB, totalB := Normalize(weightsB)

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

/*
KolmogorovSmirnov returns the Kolmogorov-Smirnov statistic between two
distributions over the same sorted position support: the supremum of the
absolute difference of their cumulative distribution functions, the worst
local cumulative disagreement. positions must be sorted ascending; weights are
normalized internally. It is dimensionless in [0,1]: 0 for identical shapes, 1
for two distributions with disjoint support.
*/
func KolmogorovSmirnov(positions, weightsA, weightsB []float64) float64 {
	if len(positions) == 0 || len(positions) != len(weightsA) || len(positions) != len(weightsB) {
		return math.Inf(1)
	}

	normalizedA, totalA := Normalize(weightsA)
	normalizedB, totalB := Normalize(weightsB)

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
Entropy returns the Shannon entropy of one distribution in natural units
(nats), given already-normalized weights (they need not sum to exactly 1 but
are interpreted as a probability mass). An empty or zero-total distribution
returns 0. The maximum is log(n) for a uniform distribution over n positions.
*/
func Entropy(weights []float64) float64 {
	entropy := 0.0

	for _, weight := range weights {
		if weight > 0 {
			entropy -= weight * math.Log(weight)
		}
	}

	return entropy
}

/*
Concentration returns the Herfindahl index of one distribution given
already-normalized weights: the sum of squared weights, in (0,1]. It equals
1/n for a uniform distribution over n positions and 1 for a single monopolized
position. It is the natural complement to entropy — one measures dominance,
the other disorder — and both are dimensionless shape facts.
*/
func Concentration(weights []float64) float64 {
	concentration := 0.0

	for _, weight := range weights {
		concentration += weight * weight
	}

	return concentration
}

/*
SortedPositions returns a clone of unsorted positions sorted ascending,
paired with its weights reordered to match, so callers can feed unordered book
levels once and obtain the canonical sorted representation both distance
functions and the CDF statistic require. It returns the sorted positions and
the reordered weights; a zero-length input returns empty slices.
*/
func SortedPositions(positions, weights []float64) ([]float64, []float64) {
	if len(positions) != len(weights) {
		return nil, nil
	}

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

/*
positionWeight pairs one position with its weight so a distribution can be
sorted by position without losing its mass association.
*/
type positionWeight struct {
	position float64
	weight   float64
}

/*
WeightedPoint is one (position, weight) observation of a distribution, sorted
ascending by position. It is the streaming form callers build when they already
have a sorted book: the merged-walk distance functions consume two such
streams directly, so no union, zero-padding, map, or combined snapshot is ever
materialized on a hot path.
*/
type WeightedPoint struct {
	Position float64
	Weight   float64
}

/*
Wasserstein1Pairs returns the first Wasserstein distance between two
distributions given as ascending-sorted WeightedPoint streams, by a single
merged walk of their positions. It is the same quantity as Wasserstein1 but
requires no shared pre-aligned support: the two streams' positions may differ
freely, and each side simply contributes zero mass at positions the other side
does not occupy. No union, map, or copy is allocated.
*/
func Wasserstein1Pairs(left, right []WeightedPoint) float64 {
	leftTotal := totalWeight(left)
	rightTotal := totalWeight(right)

	if leftTotal == 0 || rightTotal == 0 {
		return math.Inf(1)
	}

	_, distance, _ := mergedWalk(left, leftTotal, right, rightTotal)

	return distance
}

/*
KolmogorovSmirnovPairs returns the Kolmogorov-Smirnov statistic between two
distributions given as ascending-sorted WeightedPoint streams, by the same
single merged walk. It is the supremum of the absolute cumulative difference,
dimensionless in [0,1], requiring no shared pre-aligned support.
*/
func KolmogorovSmirnovPairs(left, right []WeightedPoint) float64 {
	leftTotal := totalWeight(left)
	rightTotal := totalWeight(right)

	if leftTotal == 0 || rightTotal == 0 {
		return math.Inf(1)
	}

	statistic, _, _ := mergedWalk(left, leftTotal, right, rightTotal)

	return statistic
}

/*
mergedWalk consumes two ascending-sorted position streams in one pass and
returns, in a single walk, the KS statistic (sup |ΔCDF|), the Wasserstein-1
distance (∫ |ΔCDF| dp), and the number of distinct positions visited. It
normalizes each side by its supplied total in place (weights are divided on the
fly), so no normalization copy is allocated. Because both streams are sorted,
the two distributions are compared exactly with no union, map, or combined
array.
*/
func mergedWalk(left []WeightedPoint, leftTotal float64, right []WeightedPoint, rightTotal float64) (float64, float64, int) {
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
			// Advance over every left point at this position (equal positions
			// collapse), adding their mass.
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

/*
totalWeight returns the sum of non-negative weights of a point stream, the
normalizer the merged walk divides by.
*/
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
ConcentrationPoints returns the Herfindahl concentration of a point stream,
normalizing its weights inline (no copy). It is the sum of squared normalized
weights, in (0,1]: 1/n for uniform mass over n points, 1 for a single point.
*/
func ConcentrationPoints(points []WeightedPoint) float64 {
	total := totalWeight(points)

	if total == 0 {
		return 0
	}

	concentration := 0.0

	for _, point := range points {
		if point.Weight > 0 {
			normalized := point.Weight / total

			concentration += normalized * normalized
		}
	}

	return concentration
}

/*
EntropyPoints returns the Shannon entropy (nats) of a point stream, normalizing
its weights inline (no copy). Zero for a single point, ln(n) for uniform mass
over n points.
*/
func EntropyPoints(points []WeightedPoint) float64 {
	total := totalWeight(points)

	if total == 0 {
		return 0
	}

	entropy := 0.0

	for _, point := range points {
		if point.Weight > 0 {
			normalized := point.Weight / total

			entropy -= normalized * math.Log(normalized)
		}
	}

	return entropy
}
