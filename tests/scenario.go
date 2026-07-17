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

/*
TradeAggression forces buy-side aggressor flow and scales trade qty from frame
at onward — the tape signature CVD and Hawkes should treat as drive/excitation.
*/
func TradeAggression(
	frames iter.Seq[Frame],
	at int,
	qtyMul float64,
) iter.Seq[Frame] {
	return func(yield func(Frame) bool) {
		index := 0

		for frame := range frames {
			shaped := frame

			if frame.Channel == "trade" && index >= at {
				shaped = shapeTradeAggression(frame, qtyMul)
			}

			index++

			if !yield(shaped) {
				return
			}
		}
	}
}

/*
BookDecay thins resting qty on both sides from frame at onward — mechanical
liquidity withdrawal for exhaust-style assertions.
*/
func BookDecay(frames iter.Seq[Frame], at int, depth float64) iter.Seq[Frame] {
	return func(yield func(Frame) bool) {
		index := 0

		for frame := range frames {
			shaped := frame

			if frame.Channel == "book" && index >= at {
				progress := math.Min(
					float64(index-at+1)/float64(max(index-at+1, 4)), 1,
				)
				fraction := math.Max(1-depth*progress, 0.05)
				shaped = shapeBookQty(frame, fraction, fraction)
			}

			index++

			if !yield(shaped) {
				return
			}
		}
	}
}

/*
BookImbalance loads the bid side and thins the ask side from frame at onward —
depthflow loaded-book pressure without inventing a second fixture language.
*/
func BookImbalance(
	frames iter.Seq[Frame],
	at int,
	bidMul float64,
	askMul float64,
) iter.Seq[Frame] {
	return func(yield func(Frame) bool) {
		index := 0

		for frame := range frames {
			shaped := frame

			if frame.Channel == "book" && index >= at {
				shaped = shapeBookQty(frame, bidMul, askMul)
			}

			index++

			if !yield(shaped) {
				return
			}
		}
	}
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

func shapeTradeAggression(frame Frame, qtyMul float64) Frame {
	var payload map[string]any

	if err := sonic.Unmarshal(frame.Payload, &payload); err != nil {
		panic(err)
	}

	for _, row := range frameRows(payload) {
		row["side"] = "buy"

		if qtyMul != 1 {
			if qty, ok := row["qty"].(float64); ok {
				row["qty"] = roundField(qty * qtyMul)
			}
		}
	}

	return Frame{
		Channel: frame.Channel,
		Type:    frame.Type,
		Payload: marshalFrame(payload),
	}
}

func shapeBookQty(frame Frame, bidMul, askMul float64) Frame {
	var payload map[string]any

	if err := sonic.Unmarshal(frame.Payload, &payload); err != nil {
		panic(err)
	}

	for _, row := range frameRows(payload) {
		scaleLevels(row, "bids", bidMul)
		scaleLevels(row, "asks", askMul)
	}

	return Frame{
		Channel: frame.Channel,
		Type:    frame.Type,
		Payload: marshalFrame(payload),
	}
}

/*
ThinSubject scales bid_qty/ask_qty for one ticker symbol so liquidity scarcity
can be falsified against a deep peer cohort.
*/
func ThinSubject(
	frames iter.Seq[Frame],
	symbol string,
	qtyMul float64,
) iter.Seq[Frame] {
	return func(yield func(Frame) bool) {
		for frame := range frames {
			shaped := frame

			if frame.Channel == "ticker" {
				shaped = shapeSubjectQty(frame, symbol, qtyMul)
			}

			if !yield(shaped) {
				return
			}
		}
	}
}

func shapeSubjectQty(frame Frame, symbol string, qtyMul float64) Frame {
	var payload map[string]any

	if err := sonic.Unmarshal(frame.Payload, &payload); err != nil {
		panic(err)
	}

	for _, row := range frameRows(payload) {
		if row["symbol"] != symbol {
			continue
		}

		scaleRowFields(row, qtyMul, []string{"bid_qty", "ask_qty"})
	}

	return Frame{
		Channel: frame.Channel,
		Type:    frame.Type,
		Payload: marshalFrame(payload),
	}
}

func scaleLevels(row map[string]any, side string, mul float64) {
	if mul == 1 {
		return
	}

	levels, ok := row[side].([]any)

	if !ok {
		return
	}

	for _, raw := range levels {
		level, ok := raw.(map[string]any)

		if !ok {
			continue
		}

		qty, ok := level["qty"].(float64)

		if !ok {
			continue
		}

		level["qty"] = roundField(math.Max(qty*mul, 0))
	}
}
