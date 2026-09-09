package algo

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
)

/* HayashiYoshida owns two typed return paths and their asynchronous overlap sum. */
type HayashiYoshida struct {
	core.PrimitiveError
	paths     [2]equation.LogReturns
	seed      *transport.IO
	current   core.Primitive
	normalize core.Primitive
}

/*
NewHayashiYoshida computes asynchronous covariance from {left,right} observation
collections. Each return contributes once to its own energy and to every strictly

	overlapping cross-product. Support counts overlaps, not independent samples.

The existing correlation primitive owns energy normalization, without clipping.
*/
func NewHayashiYoshida() *HayashiYoshida {
	return &HayashiYoshida{
		seed:      transport.NewIO(core.From(map[string]core.Primitive{})),
		normalize: correlation.NewCorrelation(),
	}
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
			estimate, err := estimator.Estimate(&estimator.paths[0], &estimator.paths[1], 0)
			estimator.Error(err)
			return estimate
		}, estimator)

	if result != nil {
		estimator.current = result
	}
	return result
}

/*
Estimate owns covariance evaluation for both direct and lagged callers. Shifting
all timestamps leaves log differences and their energies unchanged. Normalization
still belongs to the configured correlation graph.
*/
func (estimator *HayashiYoshida) Estimate(
	left, right *equation.LogReturns, lag int64,
) (map[string]core.Primitive, error) {
	if (lag > 0 && left.Through > math.MaxInt64-lag) ||
		(lag < 0 && left.From < math.MinInt64-lag) {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "hayashi-yoshida: timestamp offset overflows int64", nil))
	}
	covariance, support := estimator.Overlap(left.Intervals, right.Intervals, lag)
	fields := map[string]core.Primitive{
		"covariance": core.From(covariance), "support": core.From(support),
		"left_energy": core.From(left.Energy), "right_energy": core.From(right.Energy),
	}
	coefficient, err := transport.Evaluate[float64](estimator.normalize, core.From(fields))

	if err != nil {
		return nil, errnie.Error(err)
	}
	fields["correlation"] = core.From(coefficient)
	return fields, nil
}

/* Overlap traverses borrowed, ordered return intervals without shifted copies. */
func (estimator *HayashiYoshida) Overlap(
	left, right []equation.LogReturn, lag int64,
) (covariance, support float64) {
	leftIndex, rightIndex := 0, 0

	for leftIndex < len(left) && rightIndex < len(right) {
		leftReturn, rightReturn := left[leftIndex], right[rightIndex]
		leftReturn.From += lag
		leftReturn.To += lag

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
