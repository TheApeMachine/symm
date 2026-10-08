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

type toxicitySymbolState struct {
	hasPrev                      bool
	prevBid                      float64
	prevAsk                      float64
	prevBidQty                   float64
	prevAskQty                   float64
	prevAtNano                   float64
	cumBracketTradeQty           float64
	cumFillBid                   float64
	cumFillAsk                   float64
	bracketStartAtNano           float64
	fillFracBidBaseline          core.Primitive
	fillFracAskBaseline          core.Primitive
	withdrawalFracBidBaseline    core.Primitive
	withdrawalFracAskBaseline    core.Primitive
	retreatFracBidBaseline       core.Primitive
	retreatFracAskBaseline       core.Primitive
	replenishmentFracBidBaseline core.Primitive
	replenishmentFracAskBaseline core.Primitive
	fillFracBidVel               core.Primitive
	fillFracAskVel               core.Primitive
	withdrawalFracBidVel         core.Primitive
	withdrawalFracAskVel         core.Primitive
}

func newToxicitySymbolState() *toxicitySymbolState {
	return &toxicitySymbolState{
		fillFracBidBaseline:          adaptive.NewBaseline(adaptive.NewWindow()),
		fillFracAskBaseline:          adaptive.NewBaseline(adaptive.NewWindow()),
		withdrawalFracBidBaseline:    adaptive.NewBaseline(adaptive.NewWindow()),
		withdrawalFracAskBaseline:    adaptive.NewBaseline(adaptive.NewWindow()),
		retreatFracBidBaseline:       adaptive.NewBaseline(adaptive.NewWindow()),
		retreatFracAskBaseline:       adaptive.NewBaseline(adaptive.NewWindow()),
		replenishmentFracBidBaseline: adaptive.NewBaseline(adaptive.NewWindow()),
		replenishmentFracAskBaseline: adaptive.NewBaseline(adaptive.NewWindow()),
		fillFracBidVel:               temporal.NewVelocity(),
		fillFracAskVel:               temporal.NewVelocity(),
		withdrawalFracBidVel:         temporal.NewVelocity(),
		withdrawalFracAskVel:         temporal.NewVelocity(),
	}
}

/*
Toxicity measures touch fill attribution, residual quantities, adverse retreat,
withdrawal and replenishment at resting prices, baselines, z-scores, and velocities.
*/
type Toxicity struct {
	*core.PrimitiveError
	symbols map[string]*toxicitySymbolState
	out     [64]float64
}

func NewToxicity() core.Primitive {
	return &Toxicity{
		PrimitiveError: core.NewPrimitiveError(),
		symbols:        make(map[string]*toxicitySymbolState),
	}
}

func (op *Toxicity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			var operands [8]float64
			var count int

			for ptr := range message.Value.Next(nil) {
				if ptr != nil && count < 8 {
					operands[count] = *(*float64)(ptr)
					count++
				}
			}

			if count < 8 {
				op.Error(core.ErrShape)
				return
			}

			bid := operands[0]
			ask := operands[1]
			bidQty := operands[2]
			askQty := operands[3]
			tradePrice := operands[4]
			tradeQty := operands[5]
			sideIndicator := operands[6]
			atNano := operands[7]

			state, exists := op.symbols[symbol]

			if !exists {
				state = newToxicitySymbolState()
				op.symbols[symbol] = state
			}

			inBracket := tradePrice >= bid && tradePrice <= ask
			var matchedBid, matchedAsk float64

			if tradePrice == ask && sideIndicator > 0 {
				matchedAsk = tradeQty
			}

			if tradePrice == bid && sideIndicator < 0 {
				matchedBid = tradeQty
			}

			if inBracket {
				state.cumBracketTradeQty += tradeQty
				state.cumFillBid += matchedBid
				state.cumFillAsk += matchedAsk

				if state.bracketStartAtNano == 0 {
					state.bracketStartAtNano = atNano
				}
			}

			var touchFillFracBid, touchFillFracAsk float64

			if inBracket {
				if bidQty > 0 {
					touchFillFracBid = state.cumFillBid / bidQty
				}

				if askQty > 0 {
					touchFillFracAsk = state.cumFillAsk / askQty
				}
			}

			var bracketTimeDeltaSec float64

			if state.bracketStartAtNano > 0 && atNano > state.bracketStartAtNano {
				bracketTimeDeltaSec = (atNano - state.bracketStartAtNano) / 1e9
			}

			var touchFillRateBid, touchFillRateAsk float64

			if bracketTimeDeltaSec > 0 {
				touchFillRateBid = state.cumFillBid / bracketTimeDeltaSec
				touchFillRateAsk = state.cumFillAsk / bracketTimeDeltaSec
			}

			unfilledBid := bidQty
			unfilledAsk := askQty

			if state.hasPrev {
				unfilledBid = math.Max(state.prevBidQty-matchedBid, 0.0)
				unfilledAsk = math.Max(state.prevAskQty-matchedAsk, 0.0)
			}

			var prevBid, prevAsk, prevBidQty, prevAskQty float64
			var retreatedBid, retreatedAsk, retreatFracBid, retreatFracAsk, retreatRateBid, retreatRateAsk float64
			var netWithdrawnBid, netWithdrawnAsk, netWithdrawalFracBid, netWithdrawalFracAsk, netWithdrawalRateBid, netWithdrawalRateAsk float64
			var netReplenishedBid, netReplenishedAsk, netReplenishmentFracBid, netReplenishmentFracAsk, netReplenishmentRateBid, netReplenishmentRateAsk float64
			var logChangeBid, logChangeAsk float64

			if state.hasPrev {
				prevBid = state.prevBid
				prevAsk = state.prevAsk
				prevBidQty = state.prevBidQty
				prevAskQty = state.prevAskQty

				var stepDeltaSec float64

				if atNano > state.prevAtNano {
					stepDeltaSec = (atNano - state.prevAtNano) / 1e9
				}

				if prevBid > 0 && bid > 0 {
					logChangeBid = math.Log(bid / prevBid)
				}

				if prevAsk > 0 && ask > 0 {
					logChangeAsk = math.Log(ask / prevAsk)
				}

				if bid < prevBid {
					retreatedBid = state.prevBidQty
					retreatFracBid = 1.0

					if stepDeltaSec > 0 {
						retreatRateBid = retreatedBid / stepDeltaSec
					}
				}

				if bid == prevBid {
					if bidQty < unfilledBid {
						netWithdrawnBid = unfilledBid - bidQty

						if state.prevBidQty > 0 {
							netWithdrawalFracBid = netWithdrawnBid / state.prevBidQty
						}

						if stepDeltaSec > 0 {
							netWithdrawalRateBid = netWithdrawnBid / stepDeltaSec
						}
					}

					if bidQty > unfilledBid {
						netReplenishedBid = bidQty - unfilledBid

						if state.prevBidQty > 0 {
							netReplenishmentFracBid = netReplenishedBid / state.prevBidQty
						}

						if stepDeltaSec > 0 {
							netReplenishmentRateBid = netReplenishedBid / stepDeltaSec
						}
					}
				}

				if ask > prevAsk {
					retreatedAsk = state.prevAskQty
					retreatFracAsk = 1.0

					if stepDeltaSec > 0 {
						retreatRateAsk = retreatedAsk / stepDeltaSec
					}
				}

				if ask == prevAsk {
					if askQty < unfilledAsk {
						netWithdrawnAsk = unfilledAsk - askQty

						if state.prevAskQty > 0 {
							netWithdrawalFracAsk = netWithdrawnAsk / state.prevAskQty
						}

						if stepDeltaSec > 0 {
							netWithdrawalRateAsk = netWithdrawnAsk / stepDeltaSec
						}
					}

					if askQty > unfilledAsk {
						netReplenishedAsk = askQty - unfilledAsk

						if state.prevAskQty > 0 {
							netReplenishmentFracAsk = netReplenishedAsk / state.prevAskQty
						}

						if stepDeltaSec > 0 {
							netReplenishmentRateAsk = netReplenishedAsk / stepDeltaSec
						}
					}
				}
			}

			state.hasPrev = true
			state.prevBid = bid
			state.prevAsk = ask
			state.prevBidQty = bidQty
			state.prevAskQty = askQty
			state.prevAtNano = atNano

			fillFracBidCenter, fillFracBidScale := evalBaseline(state.fillFracBidBaseline, touchFillFracBid)
			fillFracAskCenter, fillFracAskScale := evalBaseline(state.fillFracAskBaseline, touchFillFracAsk)
			withdrawalFracBidCenter, withdrawalFracBidScale := evalBaseline(state.withdrawalFracBidBaseline, netWithdrawalFracBid)
			withdrawalFracAskCenter, withdrawalFracAskScale := evalBaseline(state.withdrawalFracAskBaseline, netWithdrawalFracAsk)
			retreatFracBidCenter, retreatFracBidScale := evalBaseline(state.retreatFracBidBaseline, retreatFracBid)
			retreatFracAskCenter, retreatFracAskScale := evalBaseline(state.retreatFracAskBaseline, retreatFracAsk)
			replenishmentFracBidCenter, _ := evalBaseline(state.replenishmentFracBidBaseline, netReplenishmentFracBid)
			replenishmentFracAskCenter, _ := evalBaseline(state.replenishmentFracAskBaseline, netReplenishmentFracAsk)

			fillFracBidDiv := touchFillFracBid - fillFracBidCenter
			fillFracAskDiv := touchFillFracAsk - fillFracAskCenter
			withdrawalFracBidDiv := netWithdrawalFracBid - withdrawalFracBidCenter
			withdrawalFracAskDiv := netWithdrawalFracAsk - withdrawalFracAskCenter

			var fillFracBidZ, fillFracAskZ, withdrawalFracBidZ, withdrawalFracAskZ, retreatFracBidZ, retreatFracAskZ float64

			if fillFracBidScale > 0 {
				fillFracBidZ = fillFracBidDiv / fillFracBidScale
			}

			if fillFracAskScale > 0 {
				fillFracAskZ = fillFracAskDiv / fillFracAskScale
			}

			if withdrawalFracBidScale > 0 {
				withdrawalFracBidZ = withdrawalFracBidDiv / withdrawalFracBidScale
			}

			if withdrawalFracAskScale > 0 {
				withdrawalFracAskZ = withdrawalFracAskDiv / withdrawalFracAskScale
			}

			if retreatFracBidScale > 0 {
				retreatFracBidZ = retreatFracBid / retreatFracBidScale
			}

			if retreatFracAskScale > 0 {
				retreatFracAskZ = retreatFracAsk / retreatFracAskScale
			}

			fillFracBidVelVal := evalVelocity(state.fillFracBidVel, touchFillFracBid, atNano)
			fillFracAskVelVal := evalVelocity(state.fillFracAskVel, touchFillFracAsk, atNano)
			withdrawalFracBidVelVal := evalVelocity(state.withdrawalFracBidVel, netWithdrawalFracBid, atNano)
			withdrawalFracAskVelVal := evalVelocity(state.withdrawalFracAskVel, netWithdrawalFracAsk, atNano)

			op.out = [64]float64{
				bid,
				ask,
				bidQty,
				askQty,
				unfilledBid,
				unfilledAsk,
				state.cumBracketTradeQty,
				matchedBid,
				matchedAsk,
				state.cumFillBid,
				state.cumFillAsk,
				touchFillFracBid,
				touchFillFracAsk,
				fillFracBidCenter,
				fillFracAskCenter,
				fillFracBidDiv,
				fillFracAskDiv,
				fillFracBidZ,
				fillFracAskZ,
				fillFracBidVelVal,
				fillFracAskVelVal,
				prevBid,
				prevAsk,
				prevBidQty,
				prevAskQty,
				logChangeBid,
				logChangeAsk,
				retreatedBid,
				retreatedAsk,
				retreatFracBid,
				retreatFracAsk,
				retreatRateBid,
				retreatRateAsk,
				netWithdrawnBid,
				netWithdrawnAsk,
				netWithdrawalFracBid,
				netWithdrawalFracAsk,
				netWithdrawalRateBid,
				netWithdrawalRateAsk,
				netReplenishedBid,
				netReplenishedAsk,
				netReplenishmentFracBid,
				netReplenishmentFracAsk,
				netReplenishmentRateBid,
				netReplenishmentRateAsk,
				touchFillRateBid,
				touchFillRateAsk,
				withdrawalFracBidCenter,
				withdrawalFracAskCenter,
				withdrawalFracBidDiv,
				withdrawalFracAskDiv,
				withdrawalFracBidZ,
				withdrawalFracAskZ,
				withdrawalFracBidVelVal,
				withdrawalFracAskVelVal,
				retreatFracBidCenter,
				retreatFracAskCenter,
				retreatFracBidZ,
				retreatFracAskZ,
				replenishmentFracBidCenter,
				replenishmentFracAskCenter,
				0.0,
				0.0,
				state.bracketStartAtNano,
			}

			for i := range op.out {
				if !yield(unsafe.Pointer(&op.out[i])) {
					return
				}
			}
		}
	}
}

func evalBaseline(baseline core.Primitive, val float64) (float64, float64) {
	var center, scale float64
	idx := 0

	for p := range baseline.Next(data.NewValue(val).Next(nil)) {
		if idx == 0 {
			center = *(*float64)(p)
		}

		if idx == 1 {
			scale = *(*float64)(p)
		}

		idx++
	}

	return center, scale
}

func evalVelocity(vel core.Primitive, val, atNano float64) float64 {
	var velocityVal float64

	for p := range vel.Next(data.NewValue(val, atNano).Next(nil)) {
		velocityVal = *(*float64)(p)
	}

	return velocityVal
}
