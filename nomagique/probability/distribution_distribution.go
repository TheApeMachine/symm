/*
Package distribution provides streaming distance and shape Primitives for
univariate distributions expressed as a weight at each of a sorted real
coordinate. It is deliberately free of market semantics: callers supply
weights (quantities, notions, probabilities) and positions (prices, spreads,
levels) and this package only answers "how far apart are two shapes" and "how
concentrated is one shape".

The canonical inputs are a position slice and a matching weight slice of equal
length, positions sorted ascending. Weights are non-negative and normalized
internally, so a distribution is always a probability mass over its positions.
Every measure is stateless across arrivals; nothing here retains state or is
causal beyond its inputs.
*/
package probability

import (
	"iter"
	"math"
	"sort"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
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

/*
DistributionNormalize owns scaling non-negative weights to a unit sum. Negative weights
are treated as zero. A zero total yields an all-zero slice and total 0;
callers must treat a zero-total distribution as empty rather than divide
through it.
*/
type DistributionNormalize struct {
	*core.PrimitiveError

	out NormalizedReading
}

/*
NewDistributionNormalize instantiates the weight-normalization Primitive.
*/
func NewDistributionNormalize() *DistributionNormalize {
	return &DistributionNormalize{PrimitiveError: core.NewPrimitiveError()}
}

/*
Next normalizes every arriving weight vector and hands over the scaled weights
with their total.
*/
func (distributionNormalize *DistributionNormalize) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*WeightsInput)(arriving)
			weights, total := normalizeWeights(input.Weights)
			distributionNormalize.out = NormalizedReading{Weights: weights, Total: total}

			if !yield(unsafe.Pointer(&distributionNormalize.out)) {
				return
			}
		}
	}
}

/*
normalize scales non-negative weights to a unit sum and returns them with the
total. Negative weights are treated as zero; a zero total returns an all-zero
slice and total 0.
*/
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
DistanceInput carries two weight vectors over one shared sorted position
support.
*/
type DistanceInput struct {
	Positions []float64
	WeightsA  []float64
	WeightsB  []float64
}

/*
Wasserstein1 owns the first Wasserstein (earth mover's) distance between two
distributions over the same sorted position support: the integral of the
absolute difference of their cumulative masses, the minimal total transport
mass times distance to morph one shape into the other. Value has the same
units as positions (before any caller normalization), and is 0 for identical
shapes.
*/
type Wasserstein1 struct {
	*core.PrimitiveError

	out float64
}

/*
NewWasserstein1 instantiates the shared-support earth-mover Primitive.
*/
func NewWasserstein1() *Wasserstein1 {
	return &Wasserstein1{PrimitiveError: core.NewPrimitiveError()}
}

/*
Next measures every arriving pair of weight vectors and hands over the
distance.
*/
func (wasserstein1 *Wasserstein1) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*DistanceInput)(arriving)

			if len(input.Positions) == 0 || len(input.Positions) != len(input.WeightsA) || len(input.Positions) != len(input.WeightsB) {
				wasserstein1.out = math.Inf(1)

				if !yield(unsafe.Pointer(&wasserstein1.out)) {
					return
				}

				continue
			}

			wasserstein1.out = wassersteinDistance(input.Positions, input.WeightsA, input.WeightsB)

			if !yield(unsafe.Pointer(&wasserstein1.out)) {
				return
			}
		}
	}
}

/*
wasserstein1 integrates the absolute cumulative-mass difference over the
shared sorted support, normalizing both weight vectors internally.
*/
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

/*
KolmogorovSmirnov owns the Kolmogorov-Smirnov statistic between two
distributions over the same sorted position support: the supremum of the
absolute difference of their cumulative distribution functions, the worst
local cumulative disagreement. It is dimensionless in [0,1]: 0 for identical
shapes, 1 for two distributions with disjoint support.
*/
type KolmogorovSmirnov struct {
	*core.PrimitiveError

	out float64
}

/*
NewKolmogorovSmirnov instantiates the shared-support KS Primitive.
*/
func NewKolmogorovSmirnov() *KolmogorovSmirnov {
	return &KolmogorovSmirnov{PrimitiveError: core.NewPrimitiveError()}
}

/*
Next measures every arriving pair of weight vectors and hands over the
statistic.
*/
func (kolmogorovSmirnov *KolmogorovSmirnov) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*DistanceInput)(arriving)

			if len(input.Positions) == 0 || len(input.Positions) != len(input.WeightsA) || len(input.Positions) != len(input.WeightsB) {
				kolmogorovSmirnov.out = math.Inf(1)

				if !yield(unsafe.Pointer(&kolmogorovSmirnov.out)) {
					return
				}

				continue
			}

			kolmogorovSmirnov.out = cumulativeDistance(input.Positions, input.WeightsA, input.WeightsB)

			if !yield(unsafe.Pointer(&kolmogorovSmirnov.out)) {
				return
			}
		}
	}
}

/*
kolmogorovSmirnov takes the supremum of the absolute cumulative difference
over the shared sorted support, normalizing both weight vectors internally.
*/
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

/*
DistributionEntropy owns the Shannon entropy of one distribution in natural units (nats).
An empty or zero-total distribution yields 0. The maximum is log(n) for a
uniform distribution over n positions.
*/
type DistributionEntropy struct {
	*core.PrimitiveError

	out float64
}

/*
NewDistributionEntropy instantiates the Shannon-entropy Primitive.
*/
func NewDistributionEntropy() *DistributionEntropy {
	return &DistributionEntropy{PrimitiveError: core.NewPrimitiveError()}
}

/*
Next scores every arriving weight vector and hands over its entropy.
*/
func (distributionEntropy *DistributionEntropy) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*ShapeInput)(arriving)
			entropy := 0.0

			for _, weight := range input.Weights {
				if weight > 0 {
					entropy -= weight * math.Log(weight)
				}
			}

			distributionEntropy.out = entropy

			if !yield(unsafe.Pointer(&distributionEntropy.out)) {
				return
			}
		}
	}
}

/*
Concentration owns the Herfindahl index of one distribution: the sum of
squared weights, in (0,1]. It equals 1/n for a uniform distribution over n
positions and 1 for a single monopolized position. It is the natural
complement to entropy — one measures dominance, the other disorder — and both
are dimensionless shape facts.
*/
type Concentration struct {
	*core.PrimitiveError

	out float64
}

/*
NewConcentration instantiates the Herfindahl-concentration Primitive.
*/
func NewConcentration() *Concentration {
	return &Concentration{PrimitiveError: core.NewPrimitiveError()}
}

/*
Next scores every arriving weight vector and hands over its concentration.
*/
func (concentration *Concentration) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*ShapeInput)(arriving)
			total := 0.0

			for _, weight := range input.Weights {
				total += weight * weight
			}

			concentration.out = total

			if !yield(unsafe.Pointer(&concentration.out)) {
				return
			}
		}
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
SortedReading is the ascending-sorted position clone with its weights
reordered to match.
*/
type SortedReading struct {
	Positions []float64
	Weights   []float64
}

/*
SortedPositions owns canonical ordering: it sorts unsorted positions ascending
and reorders their weights to match, so callers can feed unordered book levels
once and obtain the sorted representation the distance Primitives and the CDF
statistic require. A length mismatch yields empty slices.
*/
type SortedPositions struct {
	*core.PrimitiveError

	out SortedReading
}

/*
NewSortedPositions instantiates the canonical-sorting Primitive.
*/
func NewSortedPositions() *SortedPositions {
	return &SortedPositions{PrimitiveError: core.NewPrimitiveError()}
}

/*
Next sorts every arriving position/weight pair and hands over the canonical
representation.
*/
func (sortedPositions *SortedPositions) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*SortedInput)(arriving)

			if len(input.Positions) != len(input.Weights) {
				sortedPositions.out = SortedReading{}

				if !yield(unsafe.Pointer(&sortedPositions.out)) {
					return
				}

				continue
			}

			positions, weights := sortPositions(input.Positions, input.Weights)
			sortedPositions.out = SortedReading{Positions: positions, Weights: weights}

			if !yield(unsafe.Pointer(&sortedPositions.out)) {
				return
			}
		}
	}
}

/*
sortPositions returns the ascending-sorted position clone with its weights
reordered to match.
*/
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
have a sorted book: the merged-walk distance Primitives consume two such
streams directly, so no union, zero-padding, map, or combined snapshot is ever
materialized on a hot path.
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

/*
Wasserstein1Pairs owns the first Wasserstein distance between two
distributions given as ascending-sorted WeightedPoint streams, by a single
merged walk of their positions. It is the same quantity as Wasserstein1 but
requires no shared pre-aligned support: the two streams' positions may differ
freely, and each side simply contributes zero mass at positions the other side
does not occupy. No union, map, or copy is allocated.
*/
type Wasserstein1Pairs struct {
	*core.PrimitiveError

	out float64
}

/*
NewWasserstein1Pairs instantiates the merged-walk earth-mover Primitive.
*/
func NewWasserstein1Pairs() *Wasserstein1Pairs {
	return &Wasserstein1Pairs{PrimitiveError: core.NewPrimitiveError()}
}

/*
Next measures every arriving pair of streams and hands over the distance.
*/
func (wasserstein1Pairs *Wasserstein1Pairs) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*PairsInput)(arriving)
			_, distance, _ := mergedWalk(input.Left, input.Right)
			wasserstein1Pairs.out = distance

			if !yield(unsafe.Pointer(&wasserstein1Pairs.out)) {
				return
			}
		}
	}
}

/*
KolmogorovSmirnovPairs owns the Kolmogorov-Smirnov statistic between two
distributions given as ascending-sorted WeightedPoint streams, by the same
single merged walk. It is the supremum of the absolute cumulative difference,
dimensionless in [0,1], requiring no shared pre-aligned support.
*/
type KolmogorovSmirnovPairs struct {
	*core.PrimitiveError

	out float64
}

/*
NewKolmogorovSmirnovPairs instantiates the merged-walk KS Primitive.
*/
func NewKolmogorovSmirnovPairs() *KolmogorovSmirnovPairs {
	return &KolmogorovSmirnovPairs{PrimitiveError: core.NewPrimitiveError()}
}

/*
Next measures every arriving pair of streams and hands over the statistic.
*/
func (kolmogorovSmirnovPairs *KolmogorovSmirnovPairs) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*PairsInput)(arriving)
			statistic, _, _ := mergedWalk(input.Left, input.Right)
			kolmogorovSmirnovPairs.out = statistic

			if !yield(unsafe.Pointer(&kolmogorovSmirnovPairs.out)) {
				return
			}
		}
	}
}

/*
mergedWalk consumes two ascending-sorted position streams in one pass and
returns, in a single walk, the KS statistic (sup |ΔCDF|), the Wasserstein-1
distance (∫ |ΔCDF| dp), and the number of distinct positions visited. It
normalizes each side by its total on the fly, so no normalization copy is
allocated. A zero-total side is an unmeasurable shape and reports +Inf for
both measures. Because both streams are sorted, the two distributions are
compared exactly with no union, map, or combined array.
*/
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
PointsInput carries one point stream.
*/
type PointsInput struct {
	Points []WeightedPoint
}

/*
ConcentrationPoints owns the Herfindahl concentration of a point stream,
normalizing its weights inline (no copy). It is the sum of squared normalized
weights, in (0,1]: 1/n for uniform mass over n points, 1 for a single point.
*/
type ConcentrationPoints struct {
	*core.PrimitiveError

	out float64
}

/*
NewConcentrationPoints instantiates the point-stream Herfindahl Primitive.
*/
func NewConcentrationPoints() *ConcentrationPoints {
	return &ConcentrationPoints{PrimitiveError: core.NewPrimitiveError()}
}

/*
Next scores every arriving stream and hands over its concentration.
*/
func (concentrationPoints *ConcentrationPoints) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*PointsInput)(arriving)
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

			concentrationPoints.out = concentration

			if !yield(unsafe.Pointer(&concentrationPoints.out)) {
				return
			}
		}
	}
}

/*
EntropyPoints owns the Shannon entropy (nats) of a point stream, normalizing
its weights inline (no copy). Zero for a single point, ln(n) for uniform mass
over n points.
*/
type EntropyPoints struct {
	*core.PrimitiveError

	out float64
}

/*
NewEntropyPoints instantiates the point-stream entropy Primitive.
*/
func NewEntropyPoints() *EntropyPoints {
	return &EntropyPoints{PrimitiveError: core.NewPrimitiveError()}
}

/*
Next scores every arriving stream and hands over its entropy.
*/
func (entropyPoints *EntropyPoints) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*PointsInput)(arriving)
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

			entropyPoints.out = entropy

			if !yield(unsafe.Pointer(&entropyPoints.out)) {
				return
			}
		}
	}
}
