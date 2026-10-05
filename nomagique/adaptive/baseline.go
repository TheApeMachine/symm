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
Baseline owns causal moments and the adaptive mean-shift window.
It expects *data.Adapter on wire, reads its native input, updates moments,
and writes its native center and scale to the adapter.
*/
type Baseline struct {
	*core.PrimitiveError
	inputKey     string
	centerKey    string
	scaleKey     string
	moments      statistic.Moments
	all          statistic.Moments
	recent       statistic.Moments
	observations float64
	capacity     float64
}

func NewBaseline(mapping ...any) core.Primitive {
	op := &Baseline{
		PrimitiveError: core.NewPrimitiveError(),
		inputKey:       "value",
		centerKey:      "center",
		scaleKey:       "scale",
	}

	for i := 0; i < len(mapping)-1; i += 2 {
		k, okKey := mapping[i].(string)
		v, okVal := mapping[i+1].(string)
		if !okKey || !okVal {
			continue
		}

		switch k {
		case "value", "input":
			op.inputKey = v
		case "center":
			op.centerKey = v
		case "scale":
			op.scaleKey = v
		default:
			if op.inputKey == "value" {
				op.inputKey = v
			}
		}
	}

	return op
}

func (op *Baseline) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				continue
			}

			adapter := (*data.Adapter)(arriving)

			// Read native input from adapter
			var val float64
			readMap := data.NewMap("input", op.inputKey)
			for out := range adapter.Next(data.NewValue[any](readMap)) {
				entry := (*data.MetricEntry)(out)
				val = *(*float64)(unsafe.Pointer(uintptr(unsafe.Pointer(&entry.Metric)) + 16))
			}

			// Update moments
			reading := op.moments.Update(val)

			// Update mean-shift tracking
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

			if op.observations > 3 && recentCount > 1 && priorCount > 1 && variance > 0 {
				shift := MeanShift{
					Variance:     variance,
					Observations: op.observations,
					RecentCount:  recentCount,
					PriorCount:   priorCount,
				}

				if math.Abs(op.recent.Mean-op.all.Mean) > shift.Bound() {
					capacity := math.Max(1, math.Floor(op.capacity*0.5))
					shedRatio := capacity / op.capacity
					op.capacity = capacity
					op.all.Shed(shedRatio)
					op.moments.Shed(shedRatio)
					op.recent = statistic.Moments{}
				}
			}

			center := val
			if reading.Prior.Count > 0 {
				center = reading.Prior.Mean
			}
			scale := reading.Dispersion

			// Write native outputs to adapter
			writeMap := data.NewOutputMap(op.centerKey, center, op.scaleKey, scale)
			for range adapter.Next(data.NewValue[any](writeMap)) {
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
