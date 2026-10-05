package grid

import (
	"errors"
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Delta measures the rate of change for region activations.
*/
type Delta struct {
	*core.PrimitiveError
	regions  int
	previous []float64
	input    data.Map[string]
	output   data.Map[float64]
}

func NewDelta(regions int) *Delta {
	effectiveRegions := max(1, regions)
	output := data.NewOutputMap()

	if regions <= 1 {
		output.Values["level"] = 0
		output.Values["change"] = 0
		output.Values["region_id"] = 1

		return &Delta{
			PrimitiveError: core.NewPrimitiveError(),
			regions:        regions,
			previous:       make([]float64, effectiveRegions),
			input:          data.NewMap("level", "level"),
			output:         output,
		}
	}

	mapping := make([]string, 0, regions*2)

	for index := 0; index < regions; index++ {
		levelKey := fmt.Sprintf("level_%d", index)
		mapping = append(mapping, levelKey, levelKey)
		output.Values[levelKey] = 0
		output.Values[fmt.Sprintf("change_%d", index)] = 0
		output.Values[fmt.Sprintf("region_%d", index)] = float64(index + 1)
	}

	return &Delta{
		PrimitiveError: core.NewPrimitiveError(),
		regions:        regions,
		previous:       make([]float64, effectiveRegions),
		input:          data.NewMap(mapping...),
		output:         output,
	}
}

func (op *Delta) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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
				if errors.Is(err, core.ErrNotHeld) {
					op.Error(core.ErrShape)
					return
				}

				op.Error(err)
				return
			}

			if op.regions <= 1 {
				level, levelOK := values.Values["level"]

				if !levelOK {
					op.Error(core.ErrShape)
					return
				}

				change := level - op.previous[0]
				op.previous[0] = level

				op.output.Values["level"] = level
				op.output.Values["change"] = change
				op.output.Values["region_id"] = 1
			}

			if op.regions > 1 {
				for index := 0; index < op.regions; index++ {
					levelKey := fmt.Sprintf("level_%d", index)
					changeKey := fmt.Sprintf("change_%d", index)
					regionKey := fmt.Sprintf("region_%d", index)

					level, levelOK := values.Values[levelKey]

					if !levelOK {
						op.Error(core.ErrShape)
						return
					}

					change := level - op.previous[index]
					op.previous[index] = level

					op.output.Values[levelKey] = level
					op.output.Values[changeKey] = change
					op.output.Values[regionKey] = float64(index + 1)
				}
			}

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
