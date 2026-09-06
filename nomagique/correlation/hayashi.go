package correlation

import (
	"math"

	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Hayashi is the Hayashi-Yoshida (2005) asynchronous covariance primitive.
Every pair of return intervals that overlap in time contributes their
product; neither path is resampled onto an invented clock.

Left and Right are the two path slots. Shift offsets the Left path in
nanoseconds, so a lag search reuses one Hayashi across every candidate.

Step returns the correlation. The covariance, per-side variances, overlap
support and timestamp bounds are exposed as zero-cost accessors per the
multi-output Equation Encapsulation rule.

Degenerate behavior: an omitted path slot yields 0 — no path, no dependence.
*/
type Hayashi struct {
	Left  *Path
	Right *Path
	Shift types.Node

	leftReturns  []Interval
	rightReturns []Interval
	energies     []types.Number

	correlation   types.Number
	covariance    types.Number
	leftVariance  types.Number
	rightVariance types.Number
	support       int
	ready         bool
}

func (hayashi *Hayashi) Step(x types.Number) types.Number {
	hayashi.reset()

	if hayashi.Left == nil || hayashi.Right == nil {
		return 0
	}

	hayashi.leftReturns = hayashi.Left.Returns(hayashi.leftReturns)
	hayashi.rightReturns = hayashi.Right.Returns(hayashi.rightReturns)

	hayashi.leftVariance = energyOf(hayashi.leftReturns)
	hayashi.rightVariance = energyOf(hayashi.rightReturns)

	var shift int64

	if hayashi.Shift != nil {
		shift = int64(hayashi.Shift.Step(x))
	}

	hayashi.accumulate(shift)

	if hayashi.support == 0 ||
		hayashi.leftVariance <= 0 || hayashi.rightVariance <= 0 {
		return 0
	}

	scale := math.Sqrt(float64(hayashi.leftVariance * hayashi.rightVariance))

	if scale <= 0 {
		return 0
	}

	correlation := float64(hayashi.covariance) / scale

	if math.IsNaN(correlation) || math.IsInf(correlation, 0) {
		return 0
	}

	// The cumulative estimator is not guaranteed to land inside [-1, 1] on a
	// finite asynchronous sample; saturate rather than emit an impossible
	// correlation.
	hayashi.correlation = types.Number(math.Max(-1, math.Min(1, correlation)))
	hayashi.ready = true

	return hayashi.correlation
}

/*
accumulate sums the products of every overlapping pair of return intervals.
Both sides ascend in time, so the right-hand scan resumes from the first
interval that could still overlap rather than restarting.
*/
func (hayashi *Hayashi) accumulate(shift int64) {
	rightStart := 0

	for _, left := range hayashi.leftReturns {
		from := left.From + shift
		to := left.To + shift

		for rightStart < len(hayashi.rightReturns) &&
			from >= hayashi.rightReturns[rightStart].To {
			rightStart++
		}

		for index := rightStart; index < len(hayashi.rightReturns); index++ {
			right := hayashi.rightReturns[index]

			if right.From >= to {
				break
			}

			hayashi.covariance += left.Value * right.Value
			hayashi.support++
		}
	}
}

func (hayashi *Hayashi) reset() {
	hayashi.correlation = 0
	hayashi.covariance = 0
	hayashi.leftVariance = 0
	hayashi.rightVariance = 0
	hayashi.support = 0
	hayashi.ready = false
}

// Ready reports whether the last step produced a defined correlation.
func (hayashi *Hayashi) Ready() bool { return hayashi.ready }

// Correlation returns the last estimated correlation.
func (hayashi *Hayashi) Correlation() types.Number { return hayashi.correlation }

// Covariance returns the accumulated cumulative covariance.
func (hayashi *Hayashi) Covariance() types.Number { return hayashi.covariance }

// LeftVariance returns the total return energy of the Left path.
func (hayashi *Hayashi) LeftVariance() types.Number { return hayashi.leftVariance }

// RightVariance returns the total return energy of the Right path.
func (hayashi *Hayashi) RightVariance() types.Number { return hayashi.rightVariance }

// Support returns the count of overlapping return-interval pairs.
func (hayashi *Hayashi) Support() types.Number { return types.Number(hayashi.support) }

// LeftReturns returns the count of valid Left return intervals.
func (hayashi *Hayashi) LeftReturns() types.Number {
	return types.Number(len(hayashi.leftReturns))
}

// RightReturns returns the count of valid Right return intervals.
func (hayashi *Hayashi) RightReturns() types.Number {
	return types.Number(len(hayashi.rightReturns))
}

/*
LeftEnergyRate returns the median interval-normalized return energy of the
Left path: its typical return energy per second, robust to a single
outlying interval.
*/
func (hayashi *Hayashi) LeftEnergyRate() types.Number {
	if hayashi.Left == nil {
		return 0
	}

	hayashi.energies = hayashi.Left.Energies(hayashi.energies)

	return statistic.MedianReduction(hayashi.energies)
}

// RightEnergyRate returns the median interval-normalized energy of the Right path.
func (hayashi *Hayashi) RightEnergyRate() types.Number {
	if hayashi.Right == nil {
		return 0
	}

	hayashi.energies = hayashi.Right.Energies(hayashi.energies)

	return statistic.MedianReduction(hayashi.energies)
}

/*
SharedTime returns the seconds during which both paths were observed: the
intersection of their timestamp spans, floored at zero when they never
coexisted.
*/
func (hayashi *Hayashi) SharedTime() types.Number {
	if hayashi.Left == nil || hayashi.Right == nil {
		return 0
	}

	leftFrom, leftTo, hasLeft := hayashi.Left.Span()
	rightFrom, rightTo, hasRight := hayashi.Right.Span()

	if !hasLeft || !hasRight {
		return 0
	}

	overlap := float64(minimum(leftTo, rightTo) - maximum(leftFrom, rightFrom))

	if overlap <= 0 {
		return 0
	}

	return types.Number(overlap / NanosPerSecond)
}

/*
OverlapDensity returns the overlapping return pairs per second of shared
time: how densely the two paths co-sampled, as opposed to how long they
merely coexisted.
*/
func (hayashi *Hayashi) OverlapDensity() types.Number {
	shared := hayashi.SharedTime()

	if shared <= 0 {
		return 0
	}

	return hayashi.Support() / shared
}

/*
energyOf folds return intervals into their total squared energy — the
unnormalized variance the correlation denominator is built from.
*/
func energyOf(intervals []Interval) types.Number {
	var energy types.Number

	for _, interval := range intervals {
		energy += interval.Value * interval.Value
	}

	return energy
}

func minimum(a int64, b int64) int64 {
	if a < b {
		return a
	}

	return b
}

func maximum(a int64, b int64) int64 {
	if a > b {
		return a
	}

	return b
}

var _ types.Node = (*Hayashi)(nil)

/*
SupportSlot exposes this Hayashi's overlap support as a node, so a downstream
stage reads it as a slot inside one composition rather than the caller
shuttling the value between stages.
*/
func (hayashi *Hayashi) SupportSlot() types.Node { return supportSlot{hayashi} }

type supportSlot struct{ hayashi *Hayashi }

func (slot supportSlot) Step(types.Number) types.Number {
	return slot.hayashi.Support()
}
