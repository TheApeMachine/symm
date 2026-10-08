package distribution

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/temporal"
)

type liquiditySymbolState struct {
	bidBaseline    core.Primitive
	askBaseline    core.Primitive
	spreadBaseline core.Primitive
	bidVel         core.Primitive
	askVel         core.Primitive
	spreadVel      core.Primitive
}

func newLiquiditySymbolState() *liquiditySymbolState {
	return &liquiditySymbolState{
		bidBaseline:    adaptive.NewBaseline(adaptive.NewWindow()),
		askBaseline:    adaptive.NewBaseline(adaptive.NewWindow()),
		spreadBaseline: adaptive.NewBaseline(adaptive.NewWindow()),
		bidVel:         temporal.NewVelocity(),
		askVel:         temporal.NewVelocity(),
		spreadVel:      temporal.NewVelocity(),
	}
}

/*
Liquidity measures displayed executable capacity at the bid and ask, cost
geometry of the touch, causal baselines, divergences, z-scores, velocities,
and trajectory recurrence.
*/
type Liquidity struct {
	*core.PrimitiveError
	symbols map[string]*liquiditySymbolState
	out     [31]float64
}

func NewLiquidity() core.Primitive {
	return &Liquidity{
		PrimitiveError: core.NewPrimitiveError(),
		symbols:        make(map[string]*liquiditySymbolState),
	}
}

func (op *Liquidity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			message := (*data.Message)(arriving)
			symbol := message.Key

			if symbol == "" {
				op.Error(core.ErrDomain)
				return
			}

			var vals [5]float64
			var count int

			for ptr := range message.Value.Next(nil) {
				if ptr != nil && count < 5 {
					vals[count] = *(*float64)(ptr)
					count++
				}
			}

			if count < 5 {
				op.Error(core.ErrShape)
				return
			}

			bid := vals[0]
			ask := vals[1]
			bidQty := vals[2]
			askQty := vals[3]
			nano := vals[4]

			state, exists := op.symbols[symbol]

			if !exists {
				state = newLiquiditySymbolState()
				op.symbols[symbol] = state
			}

			bidNotional := bid * bidQty
			askNotional := ask * askQty
			midpoint := (bid + ask) / 2.0
			spread := ask - bid
			relativeSpread := spread / midpoint
			twoSidedNotional := math.Min(bidNotional, askNotional)
			total := bidNotional + askNotional
			imbalance := (bidNotional - askNotional) / total

			var bidCenter, bidScale float64
			idx := 0

			for p := range state.bidBaseline.Next(data.NewValue(bidNotional).Next(nil)) {
				if idx == 0 {
					bidCenter = *(*float64)(p)
				}

				if idx == 1 {
					bidScale = *(*float64)(p)
				}

				idx++
			}

			var askCenter, askScale float64
			idx = 0

			for p := range state.askBaseline.Next(data.NewValue(askNotional).Next(nil)) {
				if idx == 0 {
					askCenter = *(*float64)(p)
				}

				if idx == 1 {
					askScale = *(*float64)(p)
				}

				idx++
			}

			var spreadCenter, spreadScale float64
			idx = 0

			for p := range state.spreadBaseline.Next(data.NewValue(relativeSpread).Next(nil)) {
				if idx == 0 {
					spreadCenter = *(*float64)(p)
				}

				if idx == 1 {
					spreadScale = *(*float64)(p)
				}

				idx++
			}

			bidDiv := bidNotional - bidCenter
			askDiv := askNotional - askCenter
			spreadDiv := relativeSpread - spreadCenter

			var bidRatio, askRatio, spreadRatio float64

			if bidCenter > 0 {
				bidRatio = bidNotional / bidCenter
			}

			if askCenter > 0 {
				askRatio = askNotional / askCenter
			}

			if spreadCenter > 0 {
				spreadRatio = relativeSpread / spreadCenter
			}

			var bidZ, askZ, spreadZ float64

			if bidScale > 0 {
				bidZ = bidDiv / bidScale
			}

			if askScale > 0 {
				askZ = askDiv / askScale
			}

			if spreadScale > 0 {
				spreadZ = spreadDiv / spreadScale
			}

			var bidVelVal, askVelVal, spreadVelVal float64

			for p := range state.bidVel.Next(data.NewValue(bidDiv, nano).Next(nil)) {
				bidVelVal = *(*float64)(p)
			}

			for p := range state.askVel.Next(data.NewValue(askDiv, nano).Next(nil)) {
				askVelVal = *(*float64)(p)
			}

			for p := range state.spreadVel.Next(data.NewValue(spreadDiv, nano).Next(nil)) {
				spreadVelVal = *(*float64)(p)
			}

			op.out = [31]float64{
				bid,
				ask,
				bidQty,
				askQty,
				bidNotional,
				askNotional,
				midpoint,
				spread,
				relativeSpread,
				twoSidedNotional,
				imbalance,
				bidCenter,
				askCenter,
				spreadCenter,
				bidRatio,
				askRatio,
				spreadRatio,
				bidDiv,
				askDiv,
				spreadDiv,
				bidScale,
				askScale,
				spreadScale,
				bidZ,
				askZ,
				spreadZ,
				bidVelVal,
				askVelVal,
				spreadVelVal,
				0.0,
				0.0,
			}

			for i := range op.out {
				if !yield(unsafe.Pointer(&op.out[i])) {
					return
				}
			}
		}
	}
}
