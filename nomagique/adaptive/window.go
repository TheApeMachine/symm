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
Window owns the all/recent moment approximation of the mean-shift policy.
It expects *data.Adapter on wire, reads its native input, updates moments,
and writes shed_ratio and capacity to the adapter.
*/
type Window struct {
	*core.PrimitiveError
	inputKey     string
	shedRatioKey string
	capacityKey  string
	all          statistic.Moments
	recent       statistic.Moments
	observations float64
	capacity     float64
}

func NewWindow(mapping ...string) core.Primitive {
	op := &Window{
		PrimitiveError: core.NewPrimitiveError(),
		inputKey:       "value",
		shedRatioKey:   "shed_ratio",
		capacityKey:    "capacity",
	}

	for i := 0; i < len(mapping)-1; i += 2 {
		switch mapping[i] {
		case "value", "input":
			op.inputKey = mapping[i+1]
		case "shed_ratio":
			op.shedRatioKey = mapping[i+1]
		case "capacity":
			op.capacityKey = mapping[i+1]
		default:
			if op.inputKey == "value" {
				op.inputKey = mapping[i+1]
			}
		}
	}

	return op
}

func (op *Window) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				continue
			}

			adapter := (*data.Adapter)(arriving)

			var val float64
			readMap := data.NewMap("input", op.inputKey)

			for out := range adapter.Next(data.NewValue[any](readMap)) {
				entry := (*data.MetricEntry)(out)
				val = *(*float64)(unsafe.Pointer(uintptr(unsafe.Pointer(&entry.Metric)) + 16))
			}

			op.observations++
			op.capacity++
			op.all.Update(val)
			op.recent.Update(val)

			if op.observations > 3 && op.recent.Count > op.capacity*0.5 {
				op.recent.Shed(0.5)
			}

			variance := op.all.Variance
			recentCount := op.recent.Count
			priorCount := op.capacity - recentCount
			shedRatio := 1.0

			if op.observations > 3 && recentCount > 1 && priorCount > 1 && variance > 0 {
				shift := MeanShift{
					Variance:     variance,
					Observations: op.observations,
					RecentCount:  recentCount,
					PriorCount:   priorCount,
				}

				if math.Abs(op.recent.Mean-op.all.Mean) > shift.Bound() {
					capacity := math.Max(1, math.Floor(op.capacity*0.5))
					shedRatio = capacity / op.capacity
					op.capacity = capacity
					op.all.Shed(shedRatio)
					op.recent = statistic.Moments{}
				}
			}

			writeMap := data.NewOutputMap(op.shedRatioKey, shedRatio, op.capacityKey, op.capacity)
			for range adapter.Next(data.NewValue[any](writeMap)) {
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
