package grid

import (
	"errors"
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

const FormatVersion float64 = 1.0

/*
Primitive projects input channels across associative regions, tracks region
activation deltas, and encodes them into ternary condition tokens.
*/
type Primitive struct {
	*core.PrimitiveError
	regions  int
	channels int
	weights  []float64
	previous []float64
	input    data.Map[string]
	output   data.Map[float64]
}

func NewPrimitive(regions, channels int, weights []float64) *Primitive {
	output := data.NewOutputMap()
	mapping := make([]string, 0, channels*2)

	for index := 0; index < channels; index++ {
		key := fmt.Sprintf("channel_%d", index)
		mapping = append(mapping, key, key)
	}

	for index := 0; index < regions; index++ {
		output.Values[fmt.Sprintf("token_%d", index)] = 0
		output.Values[fmt.Sprintf("level_%d", index)] = 0
		output.Values[fmt.Sprintf("change_%d", index)] = 0
	}

	prim := &Primitive{
		PrimitiveError: core.NewPrimitiveError(),
		regions:        regions,
		channels:       channels,
		weights:        weights,
		previous:       make([]float64, regions),
		input:          data.NewMap(mapping...),
		output:         output,
	}

	if len(weights) != regions*channels {
		prim.Error(core.ErrShape)
	}

	return prim
}

func (op *Primitive) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			if len(op.weights) != op.regions*op.channels {
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

			channelValues := make([]float64, op.channels)

			for channelIndex := 0; channelIndex < op.channels; channelIndex++ {
				key := fmt.Sprintf("channel_%d", channelIndex)
				channelValue, channelOK := values.Values[key]

				if !channelOK {
					op.Error(core.ErrShape)
					return
				}

				channelValues[channelIndex] = channelValue
			}

			for regionIndex := 0; regionIndex < op.regions; regionIndex++ {
				level := 0.0
				offset := regionIndex * op.channels

				for channelIndex := 0; channelIndex < op.channels; channelIndex++ {
					level += op.weights[offset+channelIndex] * channelValues[channelIndex]
				}

				change := level - op.previous[regionIndex]
				op.previous[regionIndex] = level

				quantity := uint64(regionIndex + 1)
				state := uint64(0)

				if level > 0 {
					state |= 1
				}

				if level < 0 {
					state |= 2
				}

				if change > 0 {
					state |= 1 << 2
				}

				if change < 0 {
					state |= 2 << 2
				}

				token := uint64(1<<52 | quantity<<4 | state)

				tokenKey := fmt.Sprintf("token_%d", regionIndex)
				levelKey := fmt.Sprintf("level_%d", regionIndex)
				changeKey := fmt.Sprintf("change_%d", regionIndex)

				op.output.Values[tokenKey] = float64(token)
				op.output.Values[levelKey] = level
				op.output.Values[changeKey] = change
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
