package adaptive

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
WeightedWindow tests adjacent dyadic summaries using the support-dependent
mean-shift bound. It discards the oldest prefix whose mean differs from the
suffix beyond that bound, then tests again. Stationary support grows.
*/
type WeightedWindow struct {
	*core.PrimitiveError
	buckets    [128]statistic.WeightedMoments
	counts     [128]uint64
	length     int
	shift      core.Primitive
	input      data.Map[string]
	shiftInput data.Map[string]
	output     data.Map[float64]
}

func NewWeightedWindow() core.Primitive {
	output := data.NewOutputMap()
	output.Values["capacity"] = 0
	output.Values["observations"] = 0
	output.Values["shed_ratio"] = 1
	output.Values["variance"] = 0
	output.Values["recent_count"] = 0
	output.Values["prior_count"] = 0

	return &WeightedWindow{
		PrimitiveError: core.NewPrimitiveError(),
		shift:          NewMeanShift(),
		input: data.NewMap(
			"value", "value",
			"weight", "weight",
		),
		shiftInput: data.NewMap("bound", "bound"),
		output:     output,
	}
}

func (op *WeightedWindow) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			value, valueOK := values.Values["value"]
			weight, weightOK := values.Values["weight"]

			if !valueOK || !weightOK {
				op.Error(core.ErrNotHeld)
				return
			}

			if weight <= 0 || op.length >= len(op.buckets) {
				op.Error(core.ErrDomain)
				return
			}

			op.buckets[op.length] = statistic.WeightedMoments{}
			op.counts[op.length] = 1
			op.buckets[op.length].Update(value, weight)
			op.length++

			for index := op.length - 1; index >= 2; index-- {
				if op.counts[index] != op.counts[index-2] {
					continue
				}

				op.buckets[index-2].Merge(op.buckets[index-1])
				op.counts[index-2] += op.counts[index-1]
				copy(op.buckets[index-1:], op.buckets[index:op.length])
				copy(op.counts[index-1:], op.counts[index:op.length])
				op.length--
			}

			var before statistic.WeightedMoments

			for index := 0; index < op.length; index++ {
				before.Merge(op.buckets[index])
			}

			cut := true

			for cut {
				cut = false
				var all statistic.WeightedMoments

				for index := 0; index < op.length; index++ {
					all.Merge(op.buckets[index])
				}

				var prefix statistic.WeightedMoments

				for index := 0; index < op.length-1; index++ {
					prefix.Merge(op.buckets[index])
					var suffix statistic.WeightedMoments

					for suffixIndex := index + 1; suffixIndex < op.length; suffixIndex++ {
						suffix.Merge(op.buckets[suffixIndex])
					}

					prefixSupport := prefix.Support()
					suffixSupport := suffix.Support()

					if prefixSupport < 8 || suffixSupport < 8 {
						continue
					}

					variance := all.M2 / (all.Mass - all.Squared/all.Mass)
					op.output.Values["variance"] = variance
					op.output.Values["observations"] = all.Support()
					op.output.Values["recent_count"] = suffixSupport
					op.output.Values["prior_count"] = prefixSupport

					for range adapter.Next(data.NewValue(op.output)) {
					}

					if err := adapter.Error(); err != nil {
						op.Error(err)
						return
					}

					for range op.shift.Next(data.NewValue(adapter)) {
					}

					if err := op.shift.Error(); err != nil {
						op.Error(err)
						return
					}

					var shift data.Map[float64]

					for pointer := range adapter.Next(data.NewValue(op.shiftInput)) {
						shift = *(*data.Map[float64])(pointer)
					}

					if err := adapter.Error(); err != nil {
						op.Error(err)
						return
					}

					bound, held := shift.Values["bound"]

					if !held {
						op.Error(core.ErrNotHeld)
						return
					}

					if math.Abs(prefix.Mean-suffix.Mean) <= bound {
						continue
					}

					length := op.length
					op.length = copy(op.buckets[:], op.buckets[index+1:length])
					copy(op.counts[:], op.counts[index+1:length])
					cut = true
					break
				}
			}

			var after statistic.WeightedMoments

			for index := 0; index < op.length; index++ {
				after.Merge(op.buckets[index])
			}

			op.output.Values["capacity"] = after.Mass
			op.output.Values["observations"] = after.Support()
			op.output.Values["shed_ratio"] = after.Mass / before.Mass

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
