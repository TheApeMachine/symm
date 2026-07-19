package tests

import (
	"iter"
	"math"
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

/*
BidFlicker multiplies ticker bid (not last, not volume) by the cycling
multipliers from frame at onward. Quote-only chatter without a print.
*/
func BidFlicker(
	frames iter.Seq[Frame],
	at int,
	bidMuls []float64,
) iter.Seq[Frame] {
	if len(bidMuls) == 0 {
		return frames
	}

	return func(yield func(Frame) bool) {
		index := 0

		for frame := range frames {
			shaped := frame

			if frame.Channel == "ticker" && index >= at {
				mul := bidMuls[(index-at)%len(bidMuls)]
				shaped = shapeTickerBid(frame, mul)
			}

			index++

			if !yield(shaped) {
				return
			}
		}
	}
}

/*
TouchCancel collapses best-bid resting qty from frame at onward while leaving
trade flow untouched — vanishing touch with no prints.
*/
func TouchCancel(
	frames iter.Seq[Frame],
	at int,
	remainFraction float64,
) iter.Seq[Frame] {
	remain := math.Max(remainFraction, 0)

	return func(yield func(Frame) bool) {
		index := 0

		for frame := range frames {
			shaped := frame

			if frame.Channel == "book" && index >= at {
				shaped = shapeTouchCancel(frame, remain)
			}

			index++

			if !yield(shaped) {
				return
			}
		}
	}
}

/*
QuoteRetreat couples TouchCancel with BidFlicker for cancel-driven bid swings.
*/
func QuoteRetreat(
	ticker iter.Seq[Frame],
	book iter.Seq[Frame],
	at int,
	bidMuls []float64,
	remainFraction float64,
) (iter.Seq[Frame], iter.Seq[Frame]) {
	return BidFlicker(ticker, at, bidMuls), TouchCancel(book, at, remainFraction)
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
