package adaptive

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/statistic"
)

type volumeBucket struct {
	statistic.WeightedMoments
	count uint64
}

/*
WeightedWindow tests adjacent dyadic summaries using MeanShift's existing
support-dependent bound. It discards the oldest prefix whose mean differs
from the suffix beyond that bound, then tests again. Stationary support grows.
Two summaries per binary level preserve candidate cuts in bounded memory;
128 slots cover the 64-bit observation counter, not a market-time horizon.
Weights are clock increments; changing their units does not change the cuts.
*/
type WeightedWindow struct {
	*core.PrimitiveError
	buckets [128]volumeBucket
	length  int
	out     WindowReading
}

func NewWeightedWindow() *WeightedWindow {
	return &WeightedWindow{PrimitiveError: core.NewPrimitiveError()}
}

func (weightedWindow *WeightedWindow) Next(input iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for pointer := range input {
			item := (*statistic.Weighted)(pointer)
			weightedWindow.buckets[weightedWindow.length] = volumeBucket{count: 1}
			weightedWindow.buckets[weightedWindow.length].Update(item.Value, item.Weight)
			weightedWindow.length++
			weightedWindow.compress()
			before := weightedWindow.total(0)

			for weightedWindow.cut() {
			}

			after := weightedWindow.total(0)
			weightedWindow.out = WindowReading{Value: item.Value, Capacity: after.Mass,
				Observations: after.Support(), ShedRatio: after.Mass / before.Mass}

			if !yield(unsafe.Pointer(&weightedWindow.out)) {
				return
			}
		}
	}
}

func (weightedWindow *WeightedWindow) total(start int) statistic.WeightedMoments {
	var total statistic.WeightedMoments

	for index := start; index < weightedWindow.length; index++ {
		total.Merge(weightedWindow.buckets[index].WeightedMoments)
	}

	return total
}

func (weightedWindow *WeightedWindow) cut() bool {
	var prefix statistic.WeightedMoments
	all := weightedWindow.total(0)

	for index := 0; index < weightedWindow.length-1; index++ {
		prefix.Merge(weightedWindow.buckets[index].WeightedMoments)
		suffix := weightedWindow.total(index + 1)

		// Each side needs two independent observations to define variance.
		if prefix.Support() <= 1 || suffix.Support() <= 1 {
			continue
		}

		variance := all.M2 / (all.Mass - all.Squared/all.Mass)
		bound := (MeanShift{Variance: variance, Observations: all.Support(),
			RecentCount: suffix.Support(), PriorCount: prefix.Support()}).Bound()

		if math.Abs(prefix.Mean-suffix.Mean) <= bound {
			continue
		}

		weightedWindow.length = copy(weightedWindow.buckets[:], weightedWindow.buckets[index+1:weightedWindow.length])
		return true
	}

	return false
}

func (weightedWindow *WeightedWindow) compress() {
	for index := weightedWindow.length - 1; index >= 2; index-- {
		if weightedWindow.buckets[index].count != weightedWindow.buckets[index-2].count {
			continue
		}

		weightedWindow.buckets[index-2].Merge(weightedWindow.buckets[index-1].WeightedMoments)
		weightedWindow.buckets[index-2].count += weightedWindow.buckets[index-1].count
		copy(weightedWindow.buckets[index-1:], weightedWindow.buckets[index:weightedWindow.length])
		weightedWindow.length--
	}
}
