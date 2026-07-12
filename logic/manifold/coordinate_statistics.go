package manifold

import (
	"math"
	"sort"
)

/*
CoordinateStatistics is one immutable robust population summary.
*/
type CoordinateStatistics struct {
	PriceScale   float64
	SizeLocation float64
	SizeScale    float64
}

/*
Measure derives robust batch statistics without depending on order identity.
*/
func (statistics *CoordinateStatistics) Measure(
	orders []*PhysicalOrder,
	midPrice float64,
	epsilon float64,
) bool {
	if len(orders) == 0 || midPrice <= 0 || epsilon <= 0 {
		return false
	}

	priceDisplacements := make([]float64, 0, len(orders))
	sizes := make([]float64, 0, len(orders))

	for _, order := range orders {
		if order == nil || order.LimitPrice <= 0 || order.Quantity <= 0 {
			return false
		}

		priceDisplacements = append(
			priceDisplacements,
			math.Abs(math.Log(order.LimitPrice/midPrice)),
		)
		sizes = append(sizes, math.Log1p(order.Quantity))
	}

	statistics.PriceScale = math.Max(median(priceDisplacements), epsilon)
	statistics.SizeLocation = median(sizes)
	deviations := make([]float64, len(sizes))

	for index, value := range sizes {
		deviations[index] = math.Abs(value - statistics.SizeLocation)
	}

	statistics.SizeScale = math.Max(median(deviations), epsilon)
	return true
}

func median(values []float64) float64 {
	ordered := append([]float64(nil), values...)
	sort.Float64s(ordered)
	middle := len(ordered) / 2

	if len(ordered)%2 == 1 {
		return ordered[middle]
	}

	return (ordered[middle-1] + ordered[middle]) / 2
}
