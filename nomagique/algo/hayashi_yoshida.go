package algo

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* HayashiYoshida owns two typed return paths and their asynchronous overlap sum. */
type HayashiYoshida struct {
	core.PrimitiveError
	paths   [2]equation.LogReturns
	seed    *transport.IO
	current core.Primitive
}

/*
NewHayashiYoshida computes asynchronous covariance from {left,right} observation
collections. Each return contributes once to its own energy and to every strictly

	overlapping cross-product. Support counts overlaps, not independent samples.

The existing correlation primitive owns energy normalization, without clipping.
*/
func NewHayashiYoshida() core.Primitive {
	return transport.NewPipe(
		transport.NewMap(&HayashiYoshida{seed: transport.NewIO(core.From(map[string]core.Primitive{}))}),
		store.NewRecord(transport.NewPipe(), transport.NewPipe(correlation.NewCorrelation(), store.NewKey("correlation"))),
	)
}

/* Next decodes one pair and advances the earlier-ending interval at each overlap. */
func (estimator *HayashiYoshida) Next(input core.Primitive) core.Primitive {
	result := core.Yield(estimator.seed, input,
		func(_ map[string]core.Primitive, fields map[string]core.Primitive) map[string]core.Primitive {
			for index, name := range [2]string{"left", "right"} {
				observations, err := core.Field[[]core.Primitive](fields, name)

				if err != nil {
					estimator.Error(err)
					return nil
				}

				if err := estimator.paths[index].Load(observations); err != nil {
					estimator.Error(err)
					return nil
				}
			}
			covariance, support := estimator.Overlap()
			return map[string]core.Primitive{
				"covariance": core.From(covariance), "support": core.From(support),
				"left_energy":  core.From(estimator.paths[0].Energy),
				"right_energy": core.From(estimator.paths[1].Energy),
			}
		}, estimator)

	if result != nil {
		estimator.current = result
	}
	return result
}

/* Overlap traverses the ordered, internally disjoint return intervals linearly. */
func (estimator *HayashiYoshida) Overlap() (covariance, support float64) {
	left, right := estimator.paths[0].Intervals, estimator.paths[1].Intervals
	leftIndex, rightIndex := 0, 0

	for leftIndex < len(left) && rightIndex < len(right) {
		leftReturn, rightReturn := left[leftIndex], right[rightIndex]

		if leftReturn.From < rightReturn.To && rightReturn.From < leftReturn.To {
			covariance += leftReturn.Value * rightReturn.Value
			support++
		}

		if leftReturn.To <= rightReturn.To {
			leftIndex++
			continue
		}
		rightIndex++
	}
	return covariance, support
}

func (estimator *HayashiYoshida) Read() any { return core.To[any](estimator.current) }
