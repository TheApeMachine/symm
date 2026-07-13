package tests

import (
	"iter"
	"math"

	"github.com/bytedance/sonic"
)

/*
Shape rewrites the price and volume fields of every ticker and trade row using
per-frame multipliers. The base stream keeps its realistic microstructure while
the multipliers impose a controllable macro trajectory on top, which is how a
recorded slice becomes a repeatable scenario.
*/
func Shape(
	frames iter.Seq[Frame],
	priceMul func(index int) float64,
	volumeMul func(index int) float64,
) iter.Seq[Frame] {
	return func(yield func(Frame) bool) {
		index := 0

		for frame := range frames {
			shaped := shapeFrame(frame, priceMul(index), volumeMul(index))
			index++

			if !yield(shaped) {
				return
			}
		}
	}
}

/*
Spike steps price and volume up from frame at onward — a vertical, high-volume
surge, the signature of a pump.
*/
func Spike(frames iter.Seq[Frame], at int, priceMul, volumeMul float64) iter.Seq[Frame] {
	return Shape(
		frames,
		func(index int) float64 {
			if index >= at {
				return priceMul
			}

			return 1
		},
		func(index int) float64 {
			if index >= at {
				return volumeMul
			}

			return 1
		},
	)
}

/*
Drawdown ramps price down to (1-depth) linearly over the first `over` frames and
holds it there — a sustained bleed that a trailing stop must eventually catch.
*/
func Drawdown(frames iter.Seq[Frame], depth float64, over int) iter.Seq[Frame] {
	return Shape(
		frames,
		func(index int) float64 {
			fraction := math.Min(float64(index)/float64(max(over, 1)), 1)

			return 1 - depth*fraction
		},
		func(int) float64 { return 1 },
	)
}

/*
Reversal holds the base trajectory until frame at, then turns price down at a
fixed rate — the momentum flip an exit thesis is meant to detect.
*/
func Reversal(frames iter.Seq[Frame], at int, ratePerFrame float64) iter.Seq[Frame] {
	return Shape(
		frames,
		func(index int) float64 {
			if index <= at {
				return 1
			}

			return math.Max(1-ratePerFrame*float64(index-at), 0.01)
		},
		func(int) float64 { return 1 },
	)
}

func shapeFrame(frame Frame, priceMul, volumeMul float64) Frame {
	if priceMul == 1 && volumeMul == 1 {
		return frame
	}

	var payload map[string]any

	if err := sonic.Unmarshal(frame.Payload, &payload); err != nil {
		panic(err)
	}

	scaleFrameFields(payload, priceMul, volumeMul, 0, 0)

	return Frame{
		Channel: frame.Channel,
		Type:    frame.Type,
		Payload: marshalFrame(payload),
	}
}
