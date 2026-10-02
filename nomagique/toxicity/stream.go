package toxicity

import (
	"errors"
	"fmt"
	"iter"
	"math"
	"time"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
TouchDispositionState holds causal historical touch geometry for disposition accounting.
*/
type TouchDispositionState struct {
	hasPrev    bool
	prevBid    float64
	prevAsk    float64
	prevBidQty float64
	prevAskQty float64
	prevTime   time.Time
}

/*
TouchDisposition measures executable touch transitions, evaluating retreats,
withdrawals, replenishments, and log price changes.
*/
type TouchDisposition struct {
	err    error
	states map[string]*TouchDispositionState
}

func NewTouchDisposition() core.Primitive {
	return &TouchDisposition{
		states: make(map[string]*TouchDispositionState),
	}
}

func (op *TouchDisposition) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m == nil || m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			bidPrice := m.GetMetric("best_price:bid").Raw
			if bidPrice == 0 {
				bidPrice = m.GetMetric("best_bid").Raw
			}
			if bidPrice == 0 {
				bidPrice = m.GetMetric("bid").Raw
			}

			askPrice := m.GetMetric("best_price:ask").Raw
			if askPrice == 0 {
				askPrice = m.GetMetric("best_ask").Raw
			}
			if askPrice == 0 {
				askPrice = m.GetMetric("ask").Raw
			}

			bidQty := m.GetMetric("touch_quantity:bid").Raw
			if bidQty == 0 {
				bidQty = m.GetMetric("bid_qty").Raw
			}

			askQty := m.GetMetric("touch_quantity:ask").Raw
			if askQty == 0 {
				askQty = m.GetMetric("ask_qty").Raw
			}

			if bidPrice <= 0 || askPrice <= 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			if bidPrice >= askPrice {
				m.Err = errnie.Err(
					errnie.Internal,
					fmt.Sprintf("toxicity: crossed touch (%f >= %f)", bidPrice, askPrice),
					nil,
				)

				if !yield(arriving) {
					return
				}

				continue
			}

			state, exists := op.states[m.Label]
			if !exists {
				state = &TouchDispositionState{}
				op.states[m.Label] = state
			}

			spread := askPrice - bidPrice
			midpoint := (bidPrice + askPrice) / 2.0

			m.WriteMetric("best_price:bid", bidPrice)
			m.WriteMetric("best_price:ask", askPrice)
			m.WriteMetric("touch_quantity:bid", bidQty)
			m.WriteMetric("touch_quantity:ask", askQty)
			m.WriteMetric("unfilled_residual_quantity:bid", bidQty)
			m.WriteMetric("unfilled_residual_quantity:ask", askQty)

			m.SetCenterScale("best_price:bid", midpoint, spread)
			m.SetCenterScale("best_price:ask", midpoint, spread)

			if state.hasPrev {
				if !state.prevTime.IsZero() && !state.prevTime.After(m.At) {
					m.From = state.prevTime
				}

				m.WriteMetric("previous_touch_quantity:bid", state.prevBidQty)
				m.WriteMetric("previous_touch_quantity:ask", state.prevAskQty)
				m.WriteMetric("previous_best_price:bid", state.prevBid)
				m.WriteMetric("previous_best_price:ask", state.prevAsk)

				m.EnsureMetadata()
				m.SetMetadata("previous_level_disposition", "touch-only")

				dt := 0.0
				if !m.From.IsZero() {
					dt = m.At.Sub(m.From).Seconds()
				}

				if state.prevBid > 0 && bidPrice > 0 {
					m.WriteMetric("touch_price_log_change:bid", math.Log(bidPrice/state.prevBid))
				}

				if state.prevAsk > 0 && askPrice > 0 {
					m.WriteMetric("touch_price_log_change:ask", math.Log(askPrice/state.prevAsk))
				}

				if bidPrice < state.prevBid {
					m.WriteMetric("retreated_quantity:bid", state.prevBidQty)
					m.WriteNormalized("retreat_fraction:bid", 1.0)
					if dt > 0 {
						m.WriteMetric("retreat_rate:bid", state.prevBidQty/dt)
					}
				}

				if bidPrice == state.prevBid {
					if bidQty < state.prevBidQty {
						withdrawn := state.prevBidQty - bidQty
						m.WriteMetric("net_withdrawn_quantity:bid", withdrawn)
						if state.prevBidQty > 0 {
							m.WriteNormalized("net_withdrawal_fraction:bid", withdrawn/state.prevBidQty)
						}
						if dt > 0 {
							m.WriteMetric("net_withdrawal_rate:bid", withdrawn/dt)
						}
					}

					if bidQty > state.prevBidQty {
						replenished := bidQty - state.prevBidQty
						m.WriteMetric("net_replenished_quantity:bid", replenished)
						if state.prevBidQty > 0 {
							m.WriteNormalized("net_replenishment_fraction:bid", replenished/state.prevBidQty)
						}
						if dt > 0 {
							m.WriteMetric("net_replenishment_rate:bid", replenished/dt)
						}
					}
				}

				if askPrice > state.prevAsk {
					m.WriteMetric("retreated_quantity:ask", state.prevAskQty)
					m.WriteNormalized("retreat_fraction:ask", 1.0)
					if dt > 0 {
						m.WriteMetric("retreat_rate:ask", state.prevAskQty/dt)
					}
				}

				if askPrice == state.prevAsk {
					if askQty < state.prevAskQty {
						withdrawn := state.prevAskQty - askQty
						m.WriteMetric("net_withdrawn_quantity:ask", withdrawn)
						if state.prevAskQty > 0 {
							m.WriteNormalized("net_withdrawal_fraction:ask", withdrawn/state.prevAskQty)
						}
						if dt > 0 {
							m.WriteMetric("net_withdrawal_rate:ask", withdrawn/dt)
						}
					}

					if askQty > state.prevAskQty {
						replenished := askQty - state.prevAskQty
						m.WriteMetric("net_replenished_quantity:ask", replenished)
						if state.prevAskQty > 0 {
							m.WriteNormalized("net_replenishment_fraction:ask", replenished/state.prevAskQty)
						}
						if dt > 0 {
							m.WriteMetric("net_replenishment_rate:ask", replenished/dt)
						}
					}
				}
			}

			state.prevTime = m.At
			state.prevBid = bidPrice
			state.prevAsk = askPrice
			state.prevBidQty = bidQty
			state.prevAskQty = askQty
			state.hasPrev = true

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *TouchDisposition) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
TradeMatchingState tracks trade fills against the active touch quotes.
*/
type TradeMatchingState struct {
	bracketQty      float64
	matchedBidQty   float64
	matchedAskQty   float64
	touchFillBidQty float64
	touchFillAskQty float64
	hasPrevTime     bool
	prevTime        time.Time
}

/*
TradeMatching attributes executed trades against the active bracket and touch levels.
*/
type TradeMatching struct {
	err    error
	states map[string]*TradeMatchingState
}

func NewTradeMatching() core.Primitive {
	return &TradeMatching{
		states: make(map[string]*TradeMatchingState),
	}
}

func (op *TradeMatching) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m == nil || m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			price := m.GetMetric("price").Raw
			qty := m.GetMetric("qty").Raw

			if price <= 0 || qty <= 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			bidPrice := m.GetMetric("best_price:bid").Raw
			if bidPrice == 0 {
				bidPrice = m.GetMetric("best_bid").Raw
			}
			if bidPrice == 0 {
				bidPrice = m.GetMetric("bid").Raw
			}

			askPrice := m.GetMetric("best_price:ask").Raw
			if askPrice == 0 {
				askPrice = m.GetMetric("best_ask").Raw
			}
			if askPrice == 0 {
				askPrice = m.GetMetric("ask").Raw
			}

			bidQty := m.GetMetric("touch_quantity:bid").Raw
			if bidQty == 0 {
				bidQty = m.GetMetric("bid_qty").Raw
			}

			askQty := m.GetMetric("touch_quantity:ask").Raw
			if askQty == 0 {
				askQty = m.GetMetric("ask_qty").Raw
			}

			state, exists := op.states[m.Label]
			if !exists {
				state = &TradeMatchingState{}
				op.states[m.Label] = state
			}

			side, _ := m.GetProvenance("side")
			inBracket := (price >= bidPrice && price <= askPrice)
			if inBracket {
				state.bracketQty += qty
			}

			var bidFillFrac, askFillFrac float64

			if side == "sell" && price == bidPrice {
				state.matchedBidQty += qty
				state.touchFillBidQty += qty
				if bidQty > 0 {
					bidFillFrac = state.touchFillBidQty / bidQty
				}
			}

			if side == "buy" && price == askPrice {
				state.matchedAskQty += qty
				state.touchFillAskQty += qty
				if askQty > 0 {
					askFillFrac = state.touchFillAskQty / askQty
				}
			}

			var bidRate, askRate float64
			var hasRate bool

			if state.hasPrevTime {
				dt := m.At.Sub(state.prevTime).Seconds()
				if dt > 0 {
					bidRate = state.touchFillBidQty / dt
					askRate = state.touchFillAskQty / dt
					hasRate = true
				}
			}

			state.prevTime = m.At
			state.hasPrevTime = true

			m.WriteMetric("bracket_trade_quantity", state.bracketQty)
			m.WriteMetric("matched_touch_trade_quantity:bid", state.matchedBidQty)
			m.WriteMetric("matched_touch_trade_quantity:ask", state.matchedAskQty)
			m.WriteMetric("touch_fill_quantity:bid", state.touchFillBidQty)
			m.WriteMetric("touch_fill_quantity:ask", state.touchFillAskQty)
			m.WriteNormalized("touch_fill_fraction:bid", bidFillFrac)
			m.WriteNormalized("touch_fill_fraction:ask", askFillFrac)

			if hasRate {
				m.WriteMetric("touch_fill_rate:bid", bidRate)
				m.WriteMetric("touch_fill_rate:ask", askRate)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *TradeMatching) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
