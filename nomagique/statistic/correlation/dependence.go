package correlation

import (
	"iter"
	"math"
	"slices"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/temporal"
)

/*
DependenceReading preserves estimator fields and the path diagnostics around one
pair.
*/
type DependenceReading struct {
	LagEstimate
	LeftReturns     float64
	RightReturns    float64
	LeftEnergyRate  float64
	RightEnergyRate float64
	Defined         bool
	SharedTime      float64
	OverlapDensity  float64
}

/*
Dependence owns the typed path diagnostics surrounding an opaque estimator.
*/
type Dependence struct {
	*core.PrimitiveError

	estimator   core.Primitive
	pathReturns core.Primitive
	out         DependenceReading
}

/*
NewDependence creates a new Dependence primitive over the supplied estimator.
*/
func NewDependence(estimator core.Primitive) *Dependence {
	return &Dependence{PrimitiveError: core.NewPrimitiveError(), estimator: estimator, pathReturns: temporal.NewPathReturns()}
}

func (dependence *Dependence) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		defer func() {

			if dependence.pathReturns != nil {
				if err := dependence.pathReturns.Error(); err != nil {
					dependence.Error(err)
				}
			}
		}()
		for arriving := range in {
			input := (*LagProfileInput)(arriving)
			left, err := decodePath(dependence.pathReturns, input.Left)

			if err != nil {
				dependence.Error(err)
				return
			}

			right, err := decodePath(dependence.pathReturns, input.Right)

			if err != nil {
				dependence.Error(err)
				return
			}

			estimate, err := estimateAt(dependence.estimator, left, right, 0)

			if err != nil {
				dependence.Error(err)
				return
			}

			shared, density := 0.0, 0.0

			if len(left.Returns) > 0 && len(right.Returns) > 0 {
				leftFrom := left.Returns[0].From
				leftThrough := left.Returns[len(left.Returns)-1].To
				rightFrom := right.Returns[0].From
				rightThrough := right.Returns[len(right.Returns)-1].To

				start := max(leftFrom, rightFrom)
				end := min(leftThrough, rightThrough)

				if end > start {
					shared = float64(end-start) / float64(time.Second)
				}
			}

			if shared > 0 {
				density = estimate.Support / shared
			}

			leftRate := medianRate(left.Returns)
			rightRate := medianRate(right.Returns)

			dependence.out = DependenceReading{
				LagEstimate:     estimate,
				LeftReturns:     float64(len(left.Returns)),
				RightReturns:    float64(len(right.Returns)),
				LeftEnergyRate:  leftRate,
				RightEnergyRate: rightRate,
				Defined:         estimate.Defined,
				SharedTime:      shared,
				OverlapDensity:  density,
			}

			if !yield(unsafe.Pointer(&dependence.out)) {
				return
			}
		}
	}
}

func medianRate(intervals []temporal.LogReturn) float64 {
	if len(intervals) == 0 {
		return math.NaN()
	}

	rates := make([]float64, len(intervals))

	for i, r := range intervals {
		elapsed := float64(r.To-r.From) / float64(time.Second)
		rates[i] = (r.Value * r.Value) / elapsed
	}

	slices.Sort(rates)
	count := len(rates)
	return (rates[(count-1)/2] + rates[count/2]) * 0.5
}
