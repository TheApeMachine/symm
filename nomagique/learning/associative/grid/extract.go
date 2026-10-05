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
Extract validates the input feature vector channels for grid projection.
*/
type Extract struct {
	*core.PrimitiveError
	channels int
	input    data.Map[string]
	output   data.Map[float64]
}

func NewExtract(channels int) *Extract {
	output := data.NewOutputMap()

	if channels <= 1 {
		output.Values["channel"] = 0

		return &Extract{
			PrimitiveError: core.NewPrimitiveError(),
			channels:       channels,
			input:          data.NewMap("channel", "channel"),
			output:         output,
		}
	}

	mapping := make([]string, 0, channels*2)

	for index := 0; index < channels; index++ {
		key := fmt.Sprintf("channel_%d", index)
		mapping = append(mapping, key, key)
		output.Values[key] = 0
	}

	return &Extract{
		PrimitiveError: core.NewPrimitiveError(),
		channels:       channels,
		input:          data.NewMap(mapping...),
		output:         output,
	}
}

func (op *Extract) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			if op.channels <= 1 {
				val, ok := values.Values["channel"]

				if !ok {
					op.Error(core.ErrShape)
					return
				}

				op.output.Values["channel"] = val
			}

			if op.channels > 1 {
				for index := 0; index < op.channels; index++ {
					key := fmt.Sprintf("channel_%d", index)
					val, ok := values.Values[key]

					if !ok {
						op.Error(core.ErrShape)
						return
					}

					op.output.Values[key] = val
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
