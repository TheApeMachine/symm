package pumpdump

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

type pickTouch struct {
	*core.PrimitiveError
	selectField func(*TouchReading) (float64, bool)
	out         float64
}

func (pick *pickTouch) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*TouchReading)(arriving)
			value, ok := pick.selectField(reading)

			if !ok {
				continue
			}

			pick.out = value

			if !yield(unsafe.Pointer(&pick.out)) {
				return
			}
		}
	}
}

func newTouchPick(selectField func(*TouchReading) (float64, bool)) *pickTouch {
	return &pickTouch{PrimitiveError: core.NewPrimitiveError(), selectField: selectField}
}

func NewBid() core.Primitive {
	return newTouchPick(func(reading *TouchReading) (float64, bool) { return reading.Bid, true })
}

func NewAsk() core.Primitive {
	return newTouchPick(func(reading *TouchReading) (float64, bool) { return reading.Ask, true })
}

func NewMidpoint() core.Primitive {
	return newTouchPick(func(reading *TouchReading) (float64, bool) { return reading.Midpoint, true })
}

func NewSpread() core.Primitive {
	return newTouchPick(func(reading *TouchReading) (float64, bool) { return reading.Spread, true })
}

func NewRelativeSpread() core.Primitive {
	return newTouchPick(func(reading *TouchReading) (float64, bool) { return reading.Relative, true })
}

func NewSpreadBaseline() core.Primitive {
	return newTouchPick(func(reading *TouchReading) (float64, bool) {
		return reading.Baseline, reading.HasBaseline
	})
}

func NewSpreadRatio() core.Primitive {
	return newTouchPick(func(reading *TouchReading) (float64, bool) {
		return reading.Ratio, reading.HasBaseline
	})
}

func NewSpreadDivergence() core.Primitive {
	return newTouchPick(func(reading *TouchReading) (float64, bool) {
		return reading.Divergence, reading.HasBaseline
	})
}

func NewSpreadZScore() core.Primitive {
	return newTouchPick(func(reading *TouchReading) (float64, bool) {
		return reading.ZScore, reading.HasBaseline
	})
}

type pickClock struct {
	*core.PrimitiveError
	selectField func(*ClockReading) (float64, bool)
	out         float64
}

func (pick *pickClock) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*ClockReading)(arriving)
			value, ok := pick.selectField(reading)

			if !ok {
				continue
			}

			pick.out = value

			if !yield(unsafe.Pointer(&pick.out)) {
				return
			}
		}
	}
}

func newClockPick(selectField func(*ClockReading) (float64, bool)) *pickClock {
	return &pickClock{PrimitiveError: core.NewPrimitiveError(), selectField: selectField}
}

func NewTradePrice() core.Primitive {
	return newClockPick(func(reading *ClockReading) (float64, bool) { return reading.Price, true })
}

func NewTradeQuantity() core.Primitive {
	return newClockPick(func(reading *ClockReading) (float64, bool) { return reading.Qty, true })
}

func NewTradeNotional() core.Primitive {
	return newClockPick(func(reading *ClockReading) (float64, bool) { return reading.Notional, true })
}

func NewTradeInterval() core.Primitive {
	return newClockPick(func(reading *ClockReading) (float64, bool) {
		return reading.Interval, reading.HasInterval
	})
}

func NewBarTarget() core.Primitive {
	return newClockPick(func(reading *ClockReading) (float64, bool) {
		return reading.Target, reading.HasTarget
	})
}

func NewBarQuantity() core.Primitive {
	return newClockPick(func(reading *ClockReading) (float64, bool) { return reading.BarQty, true })
}

func NewBarNotional() core.Primitive {
	return newClockPick(func(reading *ClockReading) (float64, bool) { return reading.BarNotional, true })
}

func NewBarTradeCount() core.Primitive {
	return newClockPick(func(reading *ClockReading) (float64, bool) { return reading.BarCount, true })
}

func NewBarDuration() core.Primitive {
	return newClockPick(func(reading *ClockReading) (float64, bool) { return reading.Duration, true })
}

func NewVolumeRate() core.Primitive {
	return newClockPick(func(reading *ClockReading) (float64, bool) {
		return reading.VolumeRate, reading.HasRates
	})
}

func NewNotionalRate() core.Primitive {
	return newClockPick(func(reading *ClockReading) (float64, bool) {
		return reading.NotionalRate, reading.HasRates
	})
}

func NewTradeRate() core.Primitive {
	return newClockPick(func(reading *ClockReading) (float64, bool) {
		return reading.TradeRate, reading.HasRates
	})
}

func NewCompletedBars() core.Primitive {
	return newClockPick(func(reading *ClockReading) (float64, bool) {
		return reading.Completed, reading.HasRates
	})
}

func NewNotionalRateBaseline() core.Primitive {
	return newClockPick(func(reading *ClockReading) (float64, bool) {
		return reading.NotionalBaseline, reading.HasNotionalBaseline
	})
}

func NewNotionalRateRatio() core.Primitive {
	return newClockPick(func(reading *ClockReading) (float64, bool) {
		return reading.NotionalRatio, reading.HasNotionalBaseline
	})
}

func NewNotionalRateDivergence() core.Primitive {
	return newClockPick(func(reading *ClockReading) (float64, bool) {
		return reading.NotionalDiv, reading.HasNotionalBaseline
	})
}

func NewNotionalRateZScore() core.Primitive {
	return newClockPick(func(reading *ClockReading) (float64, bool) {
		return reading.NotionalZ, reading.HasNotionalBaseline
	})
}
