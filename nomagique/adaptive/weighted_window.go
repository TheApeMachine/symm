package adaptive

import (
	"errors"
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
Each bucket is a WeightedMoments summary {mass, squared mass, mean, m2}.
*/
type WeightedWindow struct {
	*core.PrimitiveError
	buckets    [128][4]float64
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

			op.buckets[op.length] = [4]float64{weight, weight * weight, value, 0}
			op.counts[op.length] = 1
			op.length++

			for index := op.length - 1; index >= 2; index-- {
				if op.counts[index] != op.counts[index-2] {
					continue
				}

				merge := statistic.NewWeightedMoments()

				for pointer := range merge.Next(data.NewValue(op.buckets[index-2 : index]...)) {
					op.buckets[index-2] = *(*[4]float64)(pointer)
				}

				if err := merge.Error(); err != nil {
					op.Error(err)
					return
				}

				op.counts[index-2] += op.counts[index-1]
				copy(op.buckets[index-1:], op.buckets[index:op.length])
				copy(op.counts[index-1:], op.counts[index:op.length])
				op.length--
			}

			var before [4]float64
			beforeFold := statistic.NewWeightedMoments()

			for pointer := range beforeFold.Next(data.NewValue(op.buckets[:op.length]...)) {
				before = *(*[4]float64)(pointer)
			}

			if err := beforeFold.Error(); err != nil {
				op.Error(err)
				return
			}

			cut := true

			for cut {
				cut = false
				var all [4]float64
				allFold := statistic.NewWeightedMoments()

				for pointer := range allFold.Next(data.NewValue(op.buckets[:op.length]...)) {
					all = *(*[4]float64)(pointer)
				}

				if err := allFold.Error(); err != nil {
					op.Error(err)
					return
				}

				var prefix [4]float64
				prefixFold := statistic.NewWeightedMoments()

				for index := 0; index < op.length-1; index++ {
					for pointer := range prefixFold.Next(data.NewValue(op.buckets[index])) {
						prefix = *(*[4]float64)(pointer)
					}

					var suffix [4]float64
					suffixFold := statistic.NewWeightedMoments()

					for pointer := range suffixFold.Next(data.NewValue(op.buckets[index+1 : op.length]...)) {
						suffix = *(*[4]float64)(pointer)
					}

					if err := errors.Join(prefixFold.Error(), suffixFold.Error()); err != nil {
						op.Error(err)
						return
					}

					// Effective support is mass² / squared mass.
					prefixSupport := prefix[0] * prefix[0] / prefix[1]
					suffixSupport := suffix[0] * suffix[0] / suffix[1]

					if prefixSupport < 8 || suffixSupport < 8 {
						continue
					}

					variance := all[3] / (all[0] - all[1]/all[0])
					op.output.Values["variance"] = variance
					op.output.Values["observations"] = all[0] * all[0] / all[1]
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

					if math.Abs(prefix[2]-suffix[2]) <= bound {
						continue
					}

					length := op.length
					op.length = copy(op.buckets[:], op.buckets[index+1:length])
					copy(op.counts[:], op.counts[index+1:length])
					cut = true
					break
				}
			}

			var after [4]float64
			afterFold := statistic.NewWeightedMoments()

			for pointer := range afterFold.Next(data.NewValue(op.buckets[:op.length]...)) {
				after = *(*[4]float64)(pointer)
			}

			if err := afterFold.Error(); err != nil {
				op.Error(err)
				return
			}

			op.output.Values["capacity"] = after[0]
			op.output.Values["observations"] = after[0] * after[0] / after[1]
			op.output.Values["shed_ratio"] = after[0] / before[0]

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
