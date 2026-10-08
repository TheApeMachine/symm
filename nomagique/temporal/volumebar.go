package temporal

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
VolumeBar maintains volume-clock tape accounting and completed-bar metrics.
Operands arrive in order: [price, qty, at_nanos, midpoint].
Yields 18 values:
0: tradeNotional
1: timeDelta
2: barTarget
3: barQuantity
4: barNotional
5: barTradeCount
6: volumeBarDuration
7: volumeRate
8: notionalRate
9: tradeRate
10: completedBars
11: fromMid
12: atMid
13: midpointLogReturn
14: midpointReturnRate
15: positiveReturn
16: negativeReturn
17: outBarStartNanos
*/
type VolumeBar struct {
	*core.PrimitiveError
	hasPrev       bool
	prevAt        float64
	hasBarStart   bool
	barStart      float64
	barQuantity   float64
	barNotional   float64
	barTradeCount float64
	BarTarget     float64
	barFromMid    float64
	completedBars float64

	hasSeenAt  bool
	lastSeenAt float64
	cached     [18]float64
}

func NewVolumeBar(target ...float64) core.Primitive {
	barTarget := 1.0
	if len(target) > 0 && target[0] > 0 {
		barTarget = target[0]
	}

	return &VolumeBar{
		PrimitiveError: core.NewPrimitiveError(),
		BarTarget:      barTarget,
	}
}

func (op *VolumeBar) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values [4]float64
		idx := 0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if idx < 4 {
				values[idx] = *(*float64)(arriving)
				idx++
			}
		}

		if idx < 4 {
			return
		}

		price := values[0]
		qty := values[1]
		atNanos := values[2]
		midpoint := values[3]

		if op.hasSeenAt && atNanos == op.lastSeenAt {
			for _, val := range op.cached {
				v := val
				if !yield(unsafe.Pointer(&v)) {
					return
				}
			}
			return
		}

		var timeDelta float64
		if op.hasPrev {
			timeDelta = (atNanos - op.prevAt) / 1e9
		}
		op.hasPrev = true
		op.prevAt = atNanos

		if !op.hasBarStart {
			op.hasBarStart = true
			op.barStart = atNanos
			op.barFromMid = midpoint
		}

		tradeNotional := price * qty
		op.barQuantity += qty
		op.barNotional += tradeNotional
		op.barTradeCount += 1

		barDuration := (atNanos - op.barStart) / 1e9
		barClosed := op.barQuantity >= op.BarTarget && barDuration > 0

		outBarStart := atNanos
		var volumeBarDuration, fromMid, atMid float64
		var volumeRate, notionalRate, tradeRate float64
		var midpointLogReturn, midpointReturnRate, positiveReturn, negativeReturn float64

		if barClosed {
			op.completedBars += 1
			outBarStart = op.barStart
			volumeBarDuration = barDuration
			fromMid = op.barFromMid
			atMid = midpoint

			if volumeBarDuration > 0 {
				volumeRate = op.barQuantity / volumeBarDuration
				notionalRate = op.barNotional / volumeBarDuration
				tradeRate = op.barTradeCount / volumeBarDuration

				if fromMid > 0 && atMid > 0 {
					midpointLogReturn = math.Log(atMid / fromMid)
					midpointReturnRate = midpointLogReturn / volumeBarDuration

					if midpointLogReturn > 0 {
						positiveReturn = midpointLogReturn
					} else {
						negativeReturn = -midpointLogReturn
					}
				}
			}
		}

		curBarTarget := op.BarTarget
		curBarQty := op.barQuantity
		curBarNotional := op.barNotional
		curBarTradeCount := op.barTradeCount
		completedBars := op.completedBars

		if barClosed {
			op.barStart = atNanos
			op.barQuantity = 0
			op.barNotional = 0
			op.barTradeCount = 0
			op.barFromMid = midpoint
		}

		op.hasSeenAt = true
		op.lastSeenAt = atNanos
		op.cached = [18]float64{
			tradeNotional,
			timeDelta,
			curBarTarget,
			curBarQty,
			curBarNotional,
			curBarTradeCount,
			volumeBarDuration,
			volumeRate,
			notionalRate,
			tradeRate,
			completedBars,
			fromMid,
			atMid,
			midpointLogReturn,
			midpointReturnRate,
			positiveReturn,
			negativeReturn,
			outBarStart,
		}

		for _, val := range op.cached {
			v := val
			if !yield(unsafe.Pointer(&v)) {
				return
			}
		}
	}
}
