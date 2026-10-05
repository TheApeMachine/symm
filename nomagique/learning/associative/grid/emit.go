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
Emit gathers region condition tokens and validates them as a completed step.
*/
type Emit struct {
	*core.PrimitiveError
	regions int
	input   data.Map[string]
	output  data.Map[float64]
}

func NewEmit(regions int) *Emit {
	output := data.NewOutputMap()

	if regions <= 1 {
		output.Values["token"] = 0

		return &Emit{
			PrimitiveError: core.NewPrimitiveError(),
			regions:        regions,
			input:          data.NewMap("token", "token"),
			output:         output,
		}
	}

	mapping := make([]string, 0, regions*2)

	for index := 0; index < regions; index++ {
		key := fmt.Sprintf("token_%d", index)
		mapping = append(mapping, key, key)
		output.Values[key] = 0
	}

	return &Emit{
		PrimitiveError: core.NewPrimitiveError(),
		regions:        regions,
		input:          data.NewMap(mapping...),
		output:         output,
	}
}

func (op *Emit) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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
				val, ok := values.Values["token"]

				if !ok {
					op.Error(core.ErrShape)
					return
				}

				op.output.Values["token"] = val
			}

			if op.regions > 1 {
				for index := 0; index < op.regions; index++ {
					key := fmt.Sprintf("token_%d", index)
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
