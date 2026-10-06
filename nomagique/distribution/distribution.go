/*
Package distribution provides streaming distance and shape Primitives for
univariate distributions expressed as a weight at each of a sorted real
coordinate. It is deliberately free of market semantics: callers supply
weights (quantities, notions, probabilities) and positions (prices, spreads,
levels) and this package only answers "how far apart are two shapes" and "how
concentrated is one shape".

Shapes on the wire are anonymous numeric arrays only:

  - a weight vector is *[]float64
  - a position/weight pair is *[2][]float64 {positions, weights}
  - two weight vectors over one shared support are *[3][]float64
    {positions, weightsA, weightsB}
  - a point is [2]float64 {position, weight}; a point stream is
    *[][2]float64 and two streams are *[2][][2]float64 {left, right}

Weights are non-negative and normalized internally, so a distribution is
always a probability mass over its positions. Every measure is stateless
across arrivals.
*/
package distribution

import (
	"iter"
	"math"
	"sort"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Normalize owns scaling non-negative weights to a unit sum. Each arrival is
*[]float64 weights; it yields *[2][]float64 {normalized, {total}}. Negative
weights are treated as zero. A zero total yields an all-zero slice and total
0; callers must treat a zero-total distribution as empty rather than divide
through it.
*/
type Normalize struct {
	*core.PrimitiveError
	out [2][]float64
}

/*
NewNormalize instantiates the weight-normalization Primitive.
*/
func NewNormalize() core.Primitive {
	return &Normalize{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

/*
Next normalizes every arriving weight vector and hands over the scaled weights
with their total.
*/
func (op *Normalize) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			weights := *(*[]float64)(arriving)
			normalized := make([]float64, len(weights))
			total := 0.0

			for _, weight := range weights {
				if weight > 0 {
					total += weight
				}
			}

			if total != 0 {
				for index, weight := range weights {
					if weight > 0 {
						normalized[index] = weight / total
					}
				}
			}

			op.out = [2][]float64{normalized, {total}}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Wasserstein1 owns the first Wasserstein (earth mover's) distance between two
distributions over the same sorted position support: the integral of the
absolute difference of their cumulative masses. Each arrival is
*[3][]float64 {positions, weightsA, weightsB}; it yields *float64. An empty or
mismatched support, or a zero-total side, yields +Inf.
*/
type Wasserstein1 struct {
	*core.PrimitiveError
	left  core.Primitive
	right core.Primitive
	out   float64
}

/*
NewWasserstein1 instantiates the shared-support earth-mover Primitive.
*/
func NewWasserstein1() core.Primitive {
	return &Wasserstein1{
		PrimitiveError: core.NewPrimitiveError(),
		left:           NewNormalize(),
		right:          NewNormalize(),
	}
}

/*
Next measures every arriving pair of weight vectors and hands over the
distance.
*/
func (op *Wasserstein1) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			input := (*[3][]float64)(arriving)
			positions := input[0]
			op.out = math.Inf(1)

			if len(positions) == 0 || len(positions) != len(input[1]) || len(positions) != len(input[2]) {
				if !yield(unsafe.Pointer(&op.out)) {
					return
				}

				continue
			}

			var left, right [2][]float64

			for pointer := range op.left.Next(data.NewValue(input[1])) {
				left = *(*[2][]float64)(pointer)
			}

			for pointer := range op.right.Next(data.NewValue(input[2])) {
				right = *(*[2][]float64)(pointer)
			}

			if err := op.Error(op.left.Error(), op.right.Error()); err != nil {
				return
			}

			if left[1][0] != 0 && right[1][0] != 0 {
				cumulative := 0.0
				distance := 0.0

				for index := 0; index < len(positions)-1; index++ {
					cumulative += left[0][index] - right[0][index]
					width := positions[index+1] - positions[index]

					if width > 0 {
						distance += math.Abs(cumulative) * width
					}
				}

				op.out = distance
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
KolmogorovSmirnov owns the Kolmogorov-Smirnov statistic between two
distributions over the same sorted position support: the supremum of the
absolute difference of their cumulative distribution functions. Each arrival
is *[3][]float64 {positions, weightsA, weightsB}; it yields *float64 in [0,1].
An empty or mismatched support, or a zero-total side, yields +Inf.
*/
type KolmogorovSmirnov struct {
	*core.PrimitiveError
	left  core.Primitive
	right core.Primitive
	out   float64
}

/*
NewKolmogorovSmirnov instantiates the shared-support KS Primitive.
*/
func NewKolmogorovSmirnov() core.Primitive {
	return &KolmogorovSmirnov{
		PrimitiveError: core.NewPrimitiveError(),
		left:           NewNormalize(),
		right:          NewNormalize(),
	}
}

/*
Next measures every arriving pair of weight vectors and hands over the
statistic.
*/
func (op *KolmogorovSmirnov) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			input := (*[3][]float64)(arriving)
			positions := input[0]
			op.out = math.Inf(1)

			if len(positions) == 0 || len(positions) != len(input[1]) || len(positions) != len(input[2]) {
				if !yield(unsafe.Pointer(&op.out)) {
					return
				}

				continue
			}

			var left, right [2][]float64

			for pointer := range op.left.Next(data.NewValue(input[1])) {
				left = *(*[2][]float64)(pointer)
			}

			for pointer := range op.right.Next(data.NewValue(input[2])) {
				right = *(*[2][]float64)(pointer)
			}

			if err := op.Error(op.left.Error(), op.right.Error()); err != nil {
				return
			}

			if left[1][0] != 0 && right[1][0] != 0 {
				cumulativeLeft := 0.0
				cumulativeRight := 0.0
				statistic := 0.0

				for index := range positions {
					cumulativeLeft += left[0][index]
					cumulativeRight += right[0][index]
					statistic = math.Max(statistic, math.Abs(cumulativeLeft-cumulativeRight))
				}

				op.out = statistic
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Entropy owns the Shannon entropy of one already-normalized distribution in
nats. Each arrival is *[]float64 weights; it yields *float64. An empty
distribution yields 0; the maximum is log(n) for uniform mass over n
positions.
*/
type Entropy struct {
	*core.PrimitiveError
	out float64
}

/*
NewEntropy instantiates the Shannon-entropy Primitive.
*/
func NewEntropy() core.Primitive {
	return &Entropy{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

/*
Next scores every arriving weight vector and hands over its entropy.
*/
func (op *Entropy) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			op.out = 0

			for _, weight := range *(*[]float64)(arriving) {
				if weight > 0 {
					op.out -= weight * math.Log(weight)
				}
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Concentration owns the Herfindahl index of one already-normalized
distribution: the sum of squared weights, in (0,1]. Each arrival is
*[]float64 weights; it yields *float64.
*/
type Concentration struct {
	*core.PrimitiveError
	out float64
}

/*
NewConcentration instantiates the Herfindahl-concentration Primitive.
*/
func NewConcentration() core.Primitive {
	return &Concentration{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

/*
Next scores every arriving weight vector and hands over its concentration.
*/
func (op *Concentration) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			op.out = 0

			for _, weight := range *(*[]float64)(arriving) {
				op.out += weight * weight
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
SortedPositions owns canonical ordering. Each arrival is *[2][]float64
{positions, weights} in any order; it yields *[2][]float64 with positions
sorted ascending and weights reordered to match. A length mismatch yields
empty (nil) slices.
*/
type SortedPositions struct {
	*core.PrimitiveError
	out [2][]float64
}

/*
NewSortedPositions instantiates the canonical-sorting Primitive.
*/
func NewSortedPositions() core.Primitive {
	return &SortedPositions{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

/*
Next sorts every arriving position/weight pair and hands over the canonical
representation.
*/
func (op *SortedPositions) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			input := (*[2][]float64)(arriving)
			positions, weights := input[0], input[1]
			op.out = [2][]float64{}

			if len(positions) == len(weights) {
				order := make([]int, len(positions))

				for index := range order {
					order[index] = index
				}

				sort.SliceStable(order, func(left, right int) bool {
					return positions[order[left]] < positions[order[right]]
				})

				op.out = [2][]float64{
					make([]float64, len(order)),
					make([]float64, len(order)),
				}

				for index, source := range order {
					op.out[0][index] = positions[source]
					op.out[1][index] = weights[source]
				}
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
MergedWalk consumes two ascending-sorted point streams in a single pass. Each
arrival is *[2][][2]float64 {left, right}, every point [2]float64
{position, weight}; it yields *[3]float64 {ks, wasserstein1, distinct}: the
supremum of |ΔCDF|, the integral of |ΔCDF| dp, and the number of distinct
positions visited. Each side is normalized by its total on the fly, and each
side contributes zero mass where the other has a point it lacks, so no union,
map, or copy is materialized. A zero-total side reports +Inf for both
measures and zero distinct positions.
*/
type MergedWalk struct {
	*core.PrimitiveError
	out [3]float64
}

/*
NewMergedWalk instantiates the merged-walk Primitive.
*/
func NewMergedWalk() core.Primitive {
	return &MergedWalk{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

/*
Next walks every arriving pair of streams and hands over both measures.
*/
func (op *MergedWalk) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			input := (*[2][][2]float64)(arriving)
			left, right := input[0], input[1]
			leftTotal := 0.0
			rightTotal := 0.0

			for _, point := range left {
				if point[1] > 0 {
					leftTotal += point[1]
				}
			}

			for _, point := range right {
				if point[1] > 0 {
					rightTotal += point[1]
				}
			}

			op.out = [3]float64{math.Inf(1), math.Inf(1), 0}

			if leftTotal == 0 || rightTotal == 0 {
				if !yield(unsafe.Pointer(&op.out)) {
					return
				}

				continue
			}

			leftIndex := 0
			rightIndex := 0
			cumulativeLeft := 0.0
			cumulativeRight := 0.0
			previous := math.NaN()
			statistic := 0.0
			distance := 0.0
			distinct := 0.0

			for leftIndex < len(left) || rightIndex < len(right) {
				var position float64

				switch {
				case leftIndex >= len(left):
					position = right[rightIndex][0]
				case rightIndex >= len(right):
					position = left[leftIndex][0]
				default:
					position = math.Min(left[leftIndex][0], right[rightIndex][0])
				}

				if !math.IsNaN(previous) && position > previous {
					distance += math.Abs(cumulativeLeft-cumulativeRight) * (position - previous)
				}

				// Equal positions collapse: every point at this position on
				// either side contributes its mass before the CDFs compare.
				for leftIndex < len(left) && left[leftIndex][0] == position {
					cumulativeLeft += left[leftIndex][1] / leftTotal
					leftIndex++
				}

				for rightIndex < len(right) && right[rightIndex][0] == position {
					cumulativeRight += right[rightIndex][1] / rightTotal
					rightIndex++
				}

				statistic = math.Max(statistic, math.Abs(cumulativeLeft-cumulativeRight))
				previous = position
				distinct++
			}

			op.out = [3]float64{statistic, distance, distinct}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Wasserstein1Pairs owns the first Wasserstein distance between two
ascending-sorted point streams on free supports. It composes MergedWalk.
Each arrival is *[2][][2]float64 {left, right}; it yields *float64.
*/
type Wasserstein1Pairs struct {
	*core.PrimitiveError
	walk core.Primitive
	out  float64
}

/*
NewWasserstein1Pairs instantiates the merged-walk earth-mover Primitive.
*/
func NewWasserstein1Pairs() core.Primitive {
	return &Wasserstein1Pairs{
		PrimitiveError: core.NewPrimitiveError(),
		walk:           NewMergedWalk(),
	}
}

/*
Next measures every arriving pair of streams and hands over the distance.
*/
func (op *Wasserstein1Pairs) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for pointer := range op.walk.Next(in) {
			op.out = (*[3]float64)(pointer)[1]

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}

		op.Error(op.walk.Error())
	}
}

/*
KolmogorovSmirnovPairs owns the Kolmogorov-Smirnov statistic between two
ascending-sorted point streams on free supports. It composes MergedWalk.
Each arrival is *[2][][2]float64 {left, right}; it yields *float64 in [0,1].
*/
type KolmogorovSmirnovPairs struct {
	*core.PrimitiveError
	walk core.Primitive
	out  float64
}

/*
NewKolmogorovSmirnovPairs instantiates the merged-walk KS Primitive.
*/
func NewKolmogorovSmirnovPairs() core.Primitive {
	return &KolmogorovSmirnovPairs{
		PrimitiveError: core.NewPrimitiveError(),
		walk:           NewMergedWalk(),
	}
}

/*
Next measures every arriving pair of streams and hands over the statistic.
*/
func (op *KolmogorovSmirnovPairs) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for pointer := range op.walk.Next(in) {
			op.out = (*[3]float64)(pointer)[0]

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}

		op.Error(op.walk.Error())
	}
}

/*
ConcentrationPoints owns the Herfindahl concentration of a point stream,
normalizing its weights inline. Each arrival is *[][2]float64; it yields
*float64: 1/n for uniform mass over n points, 1 for a single point, 0 for a
zero-total stream.
*/
type ConcentrationPoints struct {
	*core.PrimitiveError
	out float64
}

/*
NewConcentrationPoints instantiates the point-stream Herfindahl Primitive.
*/
func NewConcentrationPoints() core.Primitive {
	return &ConcentrationPoints{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

/*
Next scores every arriving stream and hands over its concentration.
*/
func (op *ConcentrationPoints) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			points := *(*[][2]float64)(arriving)
			total := 0.0
			op.out = 0

			for _, point := range points {
				if point[1] > 0 {
					total += point[1]
				}
			}

			for _, point := range points {
				if total != 0 && point[1] > 0 {
					normalized := point[1] / total
					op.out += normalized * normalized
				}
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
EntropyPoints owns the Shannon entropy (nats) of a point stream, normalizing
its weights inline. Each arrival is *[][2]float64; it yields *float64: 0 for a
single point or a zero-total stream, ln(n) for uniform mass over n points.
*/
type EntropyPoints struct {
	*core.PrimitiveError
	out float64
}

/*
NewEntropyPoints instantiates the point-stream entropy Primitive.
*/
func NewEntropyPoints() core.Primitive {
	return &EntropyPoints{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

/*
Next scores every arriving stream and hands over its entropy.
*/
func (op *EntropyPoints) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			points := *(*[][2]float64)(arriving)
			total := 0.0
			op.out = 0

			for _, point := range points {
				if point[1] > 0 {
					total += point[1]
				}
			}

			for _, point := range points {
				if total != 0 && point[1] > 0 {
					normalized := point[1] / total
					op.out -= normalized * math.Log(normalized)
				}
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
