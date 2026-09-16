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

func (window *WeightedWindow) Next(input iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for pointer := range input {
			item := (*statistic.Weighted)(pointer)
			window.buckets[window.length] = volumeBucket{count: 1}
			window.buckets[window.length].Update(item.Value, item.Weight)
			window.length++
			window.compress()
			before := window.total(0)

			for window.cut() {
			}

			after := window.total(0)
			window.out = WindowReading{Value: item.Value, Capacity: after.Mass,
				Observations: after.Support(), ShedRatio: after.Mass / before.Mass}

			if !yield(unsafe.Pointer(&window.out)) {
				return
			}
		}
	}
}

func (window *WeightedWindow) total(start int) statistic.WeightedMoments {
	var total statistic.WeightedMoments

	for index := start; index < window.length; index++ {
		total.Merge(window.buckets[index].WeightedMoments)
	}

	return total
}

func (window *WeightedWindow) cut() bool {
	var prefix statistic.WeightedMoments
	all := window.total(0)

	for index := 0; index < window.length-1; index++ {
		prefix.Merge(window.buckets[index].WeightedMoments)
		suffix := window.total(index + 1)

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

		window.length = copy(window.buckets[:], window.buckets[index+1:window.length])
		return true
	}

	return false
}

func (window *WeightedWindow) compress() {
	for index := window.length - 1; index >= 2; index-- {
		if window.buckets[index].count != window.buckets[index-2].count {
			continue
		}

		window.buckets[index-2].Merge(window.buckets[index-1].WeightedMoments)
		window.buckets[index-2].count += window.buckets[index-1].count
		copy(window.buckets[index-1:], window.buckets[index:window.length])
		window.length--
	}
}
