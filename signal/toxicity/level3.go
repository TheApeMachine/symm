package toxicity

import (
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
)

type level3State struct {
	graph          *Level3Graph
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

/*
Level3 is the book-touch market entity. It maintains an online toxicity model
per symbol through explicit Primitive records and projects data.Measurement outputs.
*/
type Level3 struct {
	states     map[string]*level3State
	symbol     string
	at         time.Time
	projection *data.Projection
}

/*
NewLevel3 constructs the Level3 entity with per-symbol Primitive compositions.
*/
func NewLevel3() *Level3 {
	entity := &Level3{states: make(map[string]*level3State), projection: level3Projection()}
	entity.projection.Identity = entity.identity
	return entity
}

func (level3 *Level3) Close() error { return nil }

/*
Touch returns the last known touch for a symbol.
*/
func (level3 *Level3) Touch(symbol string) (float64, float64, float64, float64, bool) {
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

	state, found := level3.states[symbol]

	if !found {
		state = &level3State{graph: newLevel3Graph()}
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

	typed := Level3Input{
		CurBid: curBid, CurAsk: curAsk, PrevBid: prevBid, PrevAsk: prevAsk,
		CurBidQty: curBidQty, CurAskQty: curAskQty, PrevBidQty: prevBidQty, PrevAskQty: prevAskQty,
		UnfilledBid: prevBidQty, UnfilledAsk: prevAskQty,
		LogChangeBid: logChangeBid, LogChangeAsk: logChangeAsk,
		RetreatedBid: retreatedBidQty, RetreatedAsk: retreatedAskQty,
		WithdrawnBid: withdrawnBidQty, WithdrawnAsk: withdrawnAskQty,
		ReplenishedBid: replenishedBidQty, ReplenishedAsk: replenishedAskQty,
		RetreatFracBid: retreatFractionBid, RetreatFracAsk: retreatFractionAsk,
		WithFracBid: withdrawalFractionBid, WithFracAsk: withdrawalFractionAsk,
		RepFracBid: replenishmentFractionBid, RepFracAsk: replenishmentFractionAsk,
	}

	if deltaT > 0 {
		typed.RetreatRateBid = retreatedBidQty / deltaT
		typed.RetreatRateAsk = retreatedAskQty / deltaT
		typed.WithRateBid = withdrawnBidQty / deltaT
		typed.WithRateAsk = withdrawnAskQty / deltaT
		typed.RepRateBid = replenishedBidQty / deltaT
		typed.RepRateAsk = replenishedAskQty / deltaT
		typed.HasRate = true
	}

	state.prevBid = curBid
	state.prevAsk = curAsk
	state.prevBidQty = curBidQty
	state.prevAskQty = curAskQty
	state.prevSec = sec
	state.prevNsec = nsec

	fields, err := transport.Evaluate(state.graph, transport.Values(typed))
	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}
	measurement := level3.projection.Project(fields)

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
