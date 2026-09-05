package toxicity

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

type level3State struct {
	retainedBid    float64
	retainedAsk    float64
	retainedBidQty float64
	retainedAskQty float64
	hasRetainedBid bool
	hasRetainedAsk bool

	prevBid      float64
	prevAsk      float64
	prevBidQty   float64
	prevAskQty   float64
	hasPrevTouch bool
	prevSec      float64
	prevNsec     float64

	lastSec  float64
	lastNsec float64
	hasTime  bool
}

type level3Input struct {
	curBid         calculus.Constant
	curAsk         calculus.Constant
	prevBid        calculus.Constant
	prevAsk        calculus.Constant
	curBidQty      calculus.Constant
	curAskQty      calculus.Constant
	prevBidQty     calculus.Constant
	prevAskQty     calculus.Constant
	unfilledBid    calculus.Constant
	unfilledAsk    calculus.Constant
	logChangeBid   calculus.Constant
	logChangeAsk   calculus.Constant
	retreatedBid   calculus.Constant
	retreatedAsk   calculus.Constant
	withdrawnBid   calculus.Constant
	withdrawnAsk   calculus.Constant
	replenishedBid calculus.Constant
	replenishedAsk calculus.Constant
	retreatFracBid calculus.Constant
	retreatFracAsk calculus.Constant
	withFracBid    calculus.Constant
	withFracAsk    calculus.Constant
	repFracBid     calculus.Constant
	repFracAsk     calculus.Constant
	retreatRateBid calculus.Constant
	retreatRateAsk calculus.Constant
	withRateBid    calculus.Constant
	withRateAsk    calculus.Constant
	repRateBid     calculus.Constant
	repRateAsk     calculus.Constant
	hasRate        calculus.Constant
}

/*
Level3 is the book-touch market entity. It maintains an online toxicity model
per symbol via a single nomagique.Number composition and projects data.Measurement outputs.
*/
type Level3 struct {
	mu     sync.RWMutex
	number *nomagique.Pipeline

	states map[string]*level3State
	symbol string
	at     time.Time

	withBidStd *equation.CausalResidual
	withAskStd *equation.CausalResidual
	retBidStd  *equation.CausalResidual
	retAskStd  *equation.CausalResidual

	in level3Input
}

/*
NewLevel3 constructs the Level3 entity with a single inlined Number composition.
*/
func NewLevel3() *Level3 {
	level3 := &Level3{
		states: make(map[string]*level3State),
	}

	keyFn := func() string { return level3.symbol }
	level3.withBidStd = &equation.CausalResidual{Key: keyFn}
	level3.withAskStd = &equation.CausalResidual{Key: keyFn}
	level3.retBidStd = &equation.CausalResidual{Key: keyFn}
	level3.retAskStd = &equation.CausalResidual{Key: keyFn}

	in := &level3.in

	level3.number = nomagique.Number(&nmtypes.Chain{
		A: &nmtypes.Split{
			A: &nmtypes.Split{
				A: &nmtypes.Report{
					Label: "best_price:bid", Unit: "rate", Timescale: "instantaneous",
					Value: &in.curBid,
				},
				B: &nmtypes.Report{
					Label: "best_price:ask", Unit: "rate", Timescale: "instantaneous",
					Value: &in.curAsk,
				},
				C: &nmtypes.Report{
					Label: "previous_best_price:bid", Unit: "rate", Timescale: "instantaneous",
					Value: &in.prevBid,
				},
				D: &nmtypes.Report{
					Label: "previous_best_price:ask", Unit: "rate", Timescale: "instantaneous",
					Value: &in.prevAsk,
				},
			},
			B: &nmtypes.Split{
				A: &nmtypes.Report{
					Label: "touch_quantity:bid", Unit: "count", Timescale: "instantaneous",
					Value: &in.curBidQty,
				},
				B: &nmtypes.Report{
					Label: "touch_quantity:ask", Unit: "count", Timescale: "instantaneous",
					Value: &in.curAskQty,
				},
				C: &nmtypes.Report{
					Label: "previous_touch_quantity:bid", Unit: "count", Timescale: "instantaneous",
					Value: &in.prevBidQty,
				},
				D: &nmtypes.Report{
					Label: "previous_touch_quantity:ask", Unit: "count", Timescale: "instantaneous",
					Value: &in.prevAskQty,
				},
			},
			C: &nmtypes.Split{
				A: &nmtypes.Report{
					Label: "unfilled_residual_quantity:bid", Unit: "count", Timescale: "instantaneous",
					Value: &in.unfilledBid,
				},
				B: &nmtypes.Report{
					Label: "unfilled_residual_quantity:ask", Unit: "count", Timescale: "instantaneous",
					Value: &in.unfilledAsk,
				},
				C: &nmtypes.Report{
					Label: "touch_price_log_change:bid", Unit: "dimensionless", Timescale: "instantaneous",
					Value: &in.logChangeBid,
				},
				D: &nmtypes.Report{
					Label: "touch_price_log_change:ask", Unit: "dimensionless", Timescale: "instantaneous",
					Value: &in.logChangeAsk,
				},
			},
			D: &nmtypes.Split{
				A: &nmtypes.Report{
					Label: "retreated_quantity:bid", Unit: "count", Timescale: "instantaneous",
					Value: &in.retreatedBid,
				},
				B: &nmtypes.Report{
					Label: "retreated_quantity:ask", Unit: "count", Timescale: "instantaneous",
					Value: &in.retreatedAsk,
				},
				C: &nmtypes.Report{
					Label: "net_withdrawn_quantity:bid", Unit: "count", Timescale: "instantaneous",
					Value: &in.withdrawnBid,
				},
				D: &nmtypes.Report{
					Label: "net_withdrawn_quantity:ask", Unit: "count", Timescale: "instantaneous",
					Value: &in.withdrawnAsk,
				},
			},
		},
		B: &nmtypes.Split{
			A: &nmtypes.Split{
				A: &nmtypes.Report{
					Label: "net_replenished_quantity:bid", Unit: "count", Timescale: "instantaneous",
					Value: &in.replenishedBid,
				},
				B: &nmtypes.Report{
					Label: "net_replenished_quantity:ask", Unit: "count", Timescale: "instantaneous",
					Value: &in.replenishedAsk,
				},
				C: &nmtypes.Report{
					Label: "retreat_fraction:bid", Unit: "dimensionless", Timescale: "instantaneous",
					Value: &in.retreatFracBid,
				},
				D: &nmtypes.Report{
					Label: "retreat_fraction:ask", Unit: "dimensionless", Timescale: "instantaneous",
					Value: &in.retreatFracAsk,
				},
			},
			B: &nmtypes.Split{
				A: &nmtypes.Report{
					Label: "net_withdrawal_fraction:bid", Unit: "dimensionless", Timescale: "instantaneous",
					Value: &in.withFracBid,
				},
				B: &nmtypes.Report{
					Label: "net_withdrawal_fraction:ask", Unit: "dimensionless", Timescale: "instantaneous",
					Value: &in.withFracAsk,
				},
				C: &nmtypes.Report{
					Label: "net_replenishment_fraction:bid", Unit: "dimensionless", Timescale: "instantaneous",
					Value: &in.repFracBid,
				},
				D: &nmtypes.Report{
					Label: "net_replenishment_fraction:ask", Unit: "dimensionless", Timescale: "instantaneous",
					Value: &in.repFracAsk,
				},
			},
			C: &nmtypes.Split{
				A: &nmtypes.Report{
					Label: "retreat_rate:bid", Unit: "per_second", Timescale: "per_second",
					Value: &in.retreatRateBid, Defined: &in.hasRate,
				},
				B: &nmtypes.Report{
					Label: "net_withdrawal_rate:bid", Unit: "per_second", Timescale: "per_second",
					Value: &in.withRateBid, Defined: &in.hasRate,
				},
				C: &nmtypes.Report{
					Label: "net_replenishment_rate:bid", Unit: "per_second", Timescale: "per_second",
					Value: &in.repRateBid, Defined: &in.hasRate,
				},
			},
			D: &nmtypes.Split{
				A: &nmtypes.Report{
					Label: "retreat_rate:ask", Unit: "per_second", Timescale: "per_second",
					Value: &in.retreatRateAsk, Defined: &in.hasRate,
				},
				B: &nmtypes.Report{
					Label: "net_withdrawal_rate:ask", Unit: "per_second", Timescale: "per_second",
					Value: &in.withRateAsk, Defined: &in.hasRate,
				},
				C: &nmtypes.Report{
					Label: "net_replenishment_rate:ask", Unit: "per_second", Timescale: "per_second",
					Value: &in.repRateAsk, Defined: &in.hasRate,
				},
			},
		},
		C: &nmtypes.Split{
			A: &nmtypes.Chain{
				A: &in.withFracBid,
				B: &nmtypes.Labelled{
					Prefix: "withdrawal_fraction_",
					Node: &nmtypes.Labelled{
						Names: map[string]string{
							"mean":       "baseline:bid",
							"divergence": "divergence:bid",
							"zscore":     "zscore:bid",
						},
						Node: level3.withBidStd,
					},
				},
			},
			B: &nmtypes.Chain{
				A: &in.withFracAsk,
				B: &nmtypes.Labelled{
					Prefix: "withdrawal_fraction_",
					Node: &nmtypes.Labelled{
						Names: map[string]string{
							"mean":       "baseline:ask",
							"divergence": "divergence:ask",
							"zscore":     "zscore:ask",
						},
						Node: level3.withAskStd,
					},
				},
			},
			C: &nmtypes.Chain{
				A: &in.retreatFracBid,
				B: &nmtypes.Labelled{
					Prefix: "retreat_fraction_",
					Node: &nmtypes.Labelled{
						Names: map[string]string{
							"mean":   "baseline:bid",
							"zscore": "zscore:bid",
						},
						Node: level3.retBidStd,
					},
				},
			},
			D: &nmtypes.Chain{
				A: &in.retreatFracAsk,
				B: &nmtypes.Labelled{
					Prefix: "retreat_fraction_",
					Node: &nmtypes.Labelled{
						Names: map[string]string{
							"mean":   "baseline:ask",
							"zscore": "zscore:ask",
						},
						Node: level3.retAskStd,
					},
				},
			},
		},
		D: &data.Projection{
			Source:   "toxicity",
			Identity: level3.identity,
		},
	})

	return level3
}

func (level3 *Level3) Close() error { return nil }

/*
Touch returns the last known touch for a symbol.
*/
func (level3 *Level3) Touch(symbol string) (float64, float64, float64, float64, bool) {
	level3.mu.RLock()
	defer level3.mu.RUnlock()

	state, found := level3.states[symbol]

	if !found || !state.hasPrevTouch {
		return 0, 0, 0, 0, false
	}

	return state.prevBid, state.prevAsk, state.prevBidQty, state.prevAskQty, true
}

/*
Step processes a Level3Data message, tracks the book touch, computes
attribution metrics, and projects the measurement.
*/
func (level3 *Level3) Step(message kraken.Level3Data) *data.Measurement[float64] {
	bidPrice, askPrice, bidQty, askQty := level3.bestTouch(message)
	symbol := message.Symbol
	at := message.Timestamp
	sec := float64(at.Unix())
	nsec := float64(at.Nanosecond())

	level3.mu.Lock()
	defer level3.mu.Unlock()

	state, found := level3.states[symbol]

	if !found {
		state = &level3State{}
		level3.states[symbol] = state
	}

	if state.hasTime {
		if sec < state.lastSec || (sec == state.lastSec && nsec < state.lastNsec) {
			return nil
		}
	}

	state.lastSec = sec
	state.lastNsec = nsec
	state.hasTime = true

	withdrewBid := withdrawsPrice(message.Bids, state.retainedBid, state.hasRetainedBid)
	withdrewAsk := withdrawsPrice(message.Asks, state.retainedAsk, state.hasRetainedAsk)

	if bidPrice == 0 && askPrice == 0 && !withdrewBid && !withdrewAsk {
		return nil
	}

	surrenderBid := withdrewBid && bidPrice == 0
	surrenderAsk := withdrewAsk && askPrice == 0

	if surrenderBid {
		state.hasRetainedBid = false
		state.retainedBid = 0
		state.retainedBidQty = 0
	}

	if surrenderAsk {
		state.hasRetainedAsk = false
		state.retainedAsk = 0
		state.retainedAskQty = 0
	}

	if bidPrice > 0 && (!state.hasRetainedBid || bidPrice >= state.retainedBid || withdrewBid) {
		state.retainedBid = bidPrice
		state.retainedBidQty = bidQty
		state.hasRetainedBid = true
	}

	if askPrice > 0 && (!state.hasRetainedAsk || askPrice <= state.retainedAsk || withdrewAsk) {
		state.retainedAsk = askPrice
		state.retainedAskQty = askQty
		state.hasRetainedAsk = true
	}

	complete := state.hasRetainedBid && state.hasRetainedAsk
	uncrossed := complete && state.retainedBid > 0 && state.retainedBid < state.retainedAsk

	if !uncrossed {
		return nil
	}

	if !state.hasPrevTouch {
		state.prevBid = state.retainedBid
		state.prevAsk = state.retainedAsk
		state.prevBidQty = state.retainedBidQty
		state.prevAskQty = state.retainedAskQty
		state.prevSec = sec
		state.prevNsec = nsec
		state.hasPrevTouch = true
	}

	curBid := state.retainedBid
	curAsk := state.retainedAsk
	curBidQty := state.retainedBidQty
	curAskQty := state.retainedAskQty

	prevBid := state.prevBid
	prevAsk := state.prevAsk
	prevBidQty := state.prevBidQty
	prevAskQty := state.prevAskQty

	deltaT := (sec - state.prevSec) + (nsec-state.prevNsec)*1e-9

	logChangeBid := 0.0

	if prevBid > 0 && curBid > 0 {
		logChangeBid = math.Log(curBid / prevBid)
	}

	logChangeAsk := 0.0

	if prevAsk > 0 && curAsk > 0 {
		logChangeAsk = math.Log(curAsk / prevAsk)
	}

	retreatedBidQty := 0.0
	retreatFractionBid := 0.0
	withdrawnBidQty := 0.0
	withdrawalFractionBid := 0.0
	replenishedBidQty := 0.0
	replenishmentFractionBid := 0.0

	if curBid < prevBid {
		retreatedBidQty = prevBidQty
		retreatFractionBid = 1.0
	} else if curBid == prevBid {
		if curBidQty < prevBidQty {
			withdrawnBidQty = prevBidQty - curBidQty

			if prevBidQty > 0 {
				withdrawalFractionBid = withdrawnBidQty / prevBidQty
			}
		} else if curBidQty > prevBidQty {
			replenishedBidQty = curBidQty - prevBidQty

			if prevBidQty > 0 {
				replenishmentFractionBid = replenishedBidQty / prevBidQty
			}
		}
	}

	retreatedAskQty := 0.0
	retreatFractionAsk := 0.0
	withdrawnAskQty := 0.0
	withdrawalFractionAsk := 0.0
	replenishedAskQty := 0.0
	replenishmentFractionAsk := 0.0

	if curAsk > prevAsk {
		retreatedAskQty = prevAskQty
		retreatFractionAsk = 1.0
	} else if curAsk == prevAsk {
		if curAskQty < prevAskQty {
			withdrawnAskQty = prevAskQty - curAskQty

			if prevAskQty > 0 {
				withdrawalFractionAsk = withdrawnAskQty / prevAskQty
			}
		} else if curAskQty > prevAskQty {
			replenishedAskQty = curAskQty - prevAskQty

			if prevAskQty > 0 {
				replenishmentFractionAsk = replenishedAskQty / prevAskQty
			}
		}
	}

	level3.symbol = symbol
	level3.at = at

	in := &level3.in
	in.curBid.Value = nmtypes.Number(curBid)
	in.curAsk.Value = nmtypes.Number(curAsk)
	in.prevBid.Value = nmtypes.Number(prevBid)
	in.prevAsk.Value = nmtypes.Number(prevAsk)
	in.curBidQty.Value = nmtypes.Number(curBidQty)
	in.curAskQty.Value = nmtypes.Number(curAskQty)
	in.prevBidQty.Value = nmtypes.Number(prevBidQty)
	in.prevAskQty.Value = nmtypes.Number(prevAskQty)
	in.unfilledBid.Value = nmtypes.Number(prevBidQty)
	in.unfilledAsk.Value = nmtypes.Number(prevAskQty)
	in.logChangeBid.Value = nmtypes.Number(logChangeBid)
	in.logChangeAsk.Value = nmtypes.Number(logChangeAsk)

	in.retreatedBid.Value = nmtypes.Number(retreatedBidQty)
	in.retreatedAsk.Value = nmtypes.Number(retreatedAskQty)
	in.withdrawnBid.Value = nmtypes.Number(withdrawnBidQty)
	in.withdrawnAsk.Value = nmtypes.Number(withdrawnAskQty)
	in.replenishedBid.Value = nmtypes.Number(replenishedBidQty)
	in.replenishedAsk.Value = nmtypes.Number(replenishedAskQty)

	in.retreatFracBid.Value = nmtypes.Number(retreatFractionBid)
	in.retreatFracAsk.Value = nmtypes.Number(retreatFractionAsk)
	in.withFracBid.Value = nmtypes.Number(withdrawalFractionBid)
	in.withFracAsk.Value = nmtypes.Number(withdrawalFractionAsk)
	in.repFracBid.Value = nmtypes.Number(replenishmentFractionBid)
	in.repFracAsk.Value = nmtypes.Number(replenishmentFractionAsk)

	if deltaT > 0 {
		in.retreatRateBid.Value = nmtypes.Number(retreatedBidQty / deltaT)
		in.retreatRateAsk.Value = nmtypes.Number(retreatedAskQty / deltaT)
		in.withRateBid.Value = nmtypes.Number(withdrawnBidQty / deltaT)
		in.withRateAsk.Value = nmtypes.Number(withdrawnAskQty / deltaT)
		in.repRateBid.Value = nmtypes.Number(replenishedBidQty / deltaT)
		in.repRateAsk.Value = nmtypes.Number(replenishedAskQty / deltaT)
		in.hasRate.Value = 1
	} else {
		in.hasRate.Value = 0
	}

	state.prevBid = curBid
	state.prevAsk = curAsk
	state.prevBidQty = curBidQty
	state.prevAskQty = curAskQty
	state.prevSec = sec
	state.prevNsec = nsec

	level3.number.Step(1.0)
	measurement := level3.number.Measurement()

	if measurement != nil {
		measurement.Maturity = 1.0
		measurement.SNR = 0.0
	}

	return measurement
}

func (level3 *Level3) identity() (string, string, time.Time, time.Time) {
	return fmt.Sprintf("toxicity:level3:%s:%d", level3.symbol, level3.at.UnixNano()),
		level3.symbol,
		level3.at,
		level3.at
}

func (level3 *Level3) bestTouch(
	message kraken.Level3Data,
) (bidPrice, askPrice, bidQty, askQty float64) {
	for _, order := range message.Bids {
		if !order.Resting() {
			continue
		}

		if price := order.LimitPrice.Float64(); price > bidPrice {
			bidPrice = price
			bidQty = order.OrderQty.Float64()
		}
	}

	for _, order := range message.Asks {
		if !order.Resting() {
			continue
		}

		if price := order.LimitPrice.Float64(); askPrice == 0 || price < askPrice {
			askPrice = price
			askQty = order.OrderQty.Float64()
		}
	}

	return bidPrice, askPrice, bidQty, askQty
}

func withdrawsPrice(orders []kraken.Level3Order, price float64, hasPrice bool) bool {
	if !hasPrice || price == 0 {
		return false
	}

	for _, order := range orders {
		if order.Event == "delete" && order.LimitPrice != nil && order.LimitPrice.Float64() == price {
			return true
		}
	}

	return false
}
