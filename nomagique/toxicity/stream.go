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
			totalTouchQty := bidQty + askQty

			m.SetMetric("best_price:bid", data.NewMetric[float64](
				"best_price:bid",
				data.UnitPrice,
				data.TimescaleInstantaneous,
				midpoint,
				spread,
			).Write(bidPrice))
			m.SetMetric("best_price:ask", data.NewMetric[float64](
				"best_price:ask",
				data.UnitPrice,
				data.TimescaleInstantaneous,
				midpoint,
				spread,
			).Write(askPrice))
			m.SetMetric("touch_quantity:bid", data.NewMetric[float64](
				"touch_quantity:bid",
				data.UnitQuantity,
				data.TimescaleInstantaneous,
				0.0,
				totalTouchQty,
			).Write(bidQty))
			m.SetMetric("touch_quantity:ask", data.NewMetric[float64](
				"touch_quantity:ask",
				data.UnitQuantity,
				data.TimescaleInstantaneous,
				0.0,
				totalTouchQty,
			).Write(askQty))
			m.SetMetric("unfilled_residual_quantity:bid", data.NewMetric[float64](
				"unfilled_residual_quantity:bid",
				data.UnitQuantity,
				data.TimescaleInstantaneous,
				0.0,
				bidQty,
			).Write(bidQty))
			m.SetMetric("unfilled_residual_quantity:ask", data.NewMetric[float64](
				"unfilled_residual_quantity:ask",
				data.UnitQuantity,
				data.TimescaleInstantaneous,
				0.0,
				askQty,
			).Write(askQty))

			if state.hasPrev {
				if !state.prevTime.IsZero() && !state.prevTime.After(m.At) {
					m.From = state.prevTime
				}

				prevMid := (state.prevBid + state.prevAsk) / 2.0
				prevSpread := state.prevAsk - state.prevBid
				prevTotalTouchQty := state.prevBidQty + state.prevAskQty

				m.SetMetric("previous_touch_quantity:bid", data.NewMetric[float64](
					"previous_touch_quantity:bid",
					data.UnitQuantity,
					data.TimescaleInstantaneous,
					0.0,
					prevTotalTouchQty,
				).Write(state.prevBidQty))
				m.SetMetric("previous_touch_quantity:ask", data.NewMetric[float64](
					"previous_touch_quantity:ask",
					data.UnitQuantity,
					data.TimescaleInstantaneous,
					0.0,
					prevTotalTouchQty,
				).Write(state.prevAskQty))
				m.SetMetric("previous_best_price:bid", data.NewMetric[float64](
					"previous_best_price:bid",
					data.UnitPrice,
					data.TimescaleInstantaneous,
					prevMid,
					prevSpread,
				).Write(state.prevBid))
				m.SetMetric("previous_best_price:ask", data.NewMetric[float64](
					"previous_best_price:ask",
					data.UnitPrice,
					data.TimescaleInstantaneous,
					prevMid,
					prevSpread,
				).Write(state.prevAsk))

				m.EnsureMetadata()
				m.SetMetadata("previous_level_disposition", "touch-only")

				dt := 0.0
				if !m.From.IsZero() {
					dt = m.At.Sub(m.From).Seconds()
				}

				if state.prevBid > 0 && bidPrice > 0 {
					relSpreadBid := prevSpread / state.prevBid
					m.SetMetric("touch_price_log_change:bid", data.NewMetric[float64](
						"touch_price_log_change:bid",
						data.UnitDimensionless,
						data.TimescaleInstantaneous,
						0.0,
						relSpreadBid,
					).Write(math.Log(bidPrice/state.prevBid)))
				}

				if state.prevAsk > 0 && askPrice > 0 {
					relSpreadAsk := prevSpread / state.prevAsk
					m.SetMetric("touch_price_log_change:ask", data.NewMetric[float64](
						"touch_price_log_change:ask",
						data.UnitDimensionless,
						data.TimescaleInstantaneous,
						0.0,
						relSpreadAsk,
					).Write(math.Log(askPrice/state.prevAsk)))
				}

				if bidPrice < state.prevBid {
					m.SetMetric("retreated_quantity:bid", data.NewMetric[float64](
						"retreated_quantity:bid",
						data.UnitQuantity,
						data.TimescaleInstantaneous,
						0.0,
						state.prevBidQty,
					).Write(state.prevBidQty))
					m.WriteNormalized("retreat_fraction:bid", 1.0)
					if dt > 0 {
						retreatRateScale := state.prevBidQty / dt
						m.SetMetric("retreat_rate:bid", data.NewMetric[float64](
							"retreat_rate:bid",
							data.UnitRate,
							data.TimescaleInstantaneous,
							0.0,
							retreatRateScale,
						).Write(retreatRateScale))
					}
				}

				if bidPrice == state.prevBid {
					if bidQty < state.prevBidQty {
						withdrawn := state.prevBidQty - bidQty
						m.SetMetric("net_withdrawn_quantity:bid", data.NewMetric[float64](
							"net_withdrawn_quantity:bid",
							data.UnitQuantity,
							data.TimescaleInstantaneous,
							0.0,
							state.prevBidQty,
						).Write(withdrawn))
						if state.prevBidQty > 0 {
							m.WriteNormalized("net_withdrawal_fraction:bid", withdrawn/state.prevBidQty)
						}
						if dt > 0 {
							withdrawnRateScale := state.prevBidQty / dt
							m.SetMetric("net_withdrawal_rate:bid", data.NewMetric[float64](
								"net_withdrawal_rate:bid",
								data.UnitRate,
								data.TimescaleInstantaneous,
								0.0,
								withdrawnRateScale,
							).Write(withdrawn/dt))
						}
					}

					if bidQty > state.prevBidQty {
						replenished := bidQty - state.prevBidQty
						m.SetMetric("net_replenished_quantity:bid", data.NewMetric[float64](
							"net_replenished_quantity:bid",
							data.UnitQuantity,
							data.TimescaleInstantaneous,
							0.0,
							state.prevBidQty,
						).Write(replenished))
						if state.prevBidQty > 0 {
							m.WriteNormalized("net_replenishment_fraction:bid", replenished/state.prevBidQty)
						}
						if dt > 0 {
							replenishRateScale := state.prevBidQty / dt
							m.SetMetric("net_replenishment_rate:bid", data.NewMetric[float64](
								"net_replenishment_rate:bid",
								data.UnitRate,
								data.TimescaleInstantaneous,
								0.0,
								replenishRateScale,
							).Write(replenished/dt))
						}
					}
				}

				if askPrice > state.prevAsk {
					m.SetMetric("retreated_quantity:ask", data.NewMetric[float64](
						"retreated_quantity:ask",
						data.UnitQuantity,
						data.TimescaleInstantaneous,
						0.0,
						state.prevAskQty,
					).Write(state.prevAskQty))
					m.WriteNormalized("retreat_fraction:ask", 1.0)
					if dt > 0 {
						retreatRateScale := state.prevAskQty / dt
						m.SetMetric("retreat_rate:ask", data.NewMetric[float64](
							"retreat_rate:ask",
							data.UnitRate,
							data.TimescaleInstantaneous,
							0.0,
							retreatRateScale,
						).Write(retreatRateScale))
					}
				}

				if askPrice == state.prevAsk {
					if askQty < state.prevAskQty {
						withdrawn := state.prevAskQty - askQty
						m.SetMetric("net_withdrawn_quantity:ask", data.NewMetric[float64](
							"net_withdrawn_quantity:ask",
							data.UnitQuantity,
							data.TimescaleInstantaneous,
							0.0,
							state.prevAskQty,
						).Write(withdrawn))
						if state.prevAskQty > 0 {
							m.WriteNormalized("net_withdrawal_fraction:ask", withdrawn/state.prevAskQty)
						}
						if dt > 0 {
							withdrawnRateScale := state.prevAskQty / dt
							m.SetMetric("net_withdrawal_rate:ask", data.NewMetric[float64](
								"net_withdrawal_rate:ask",
								data.UnitRate,
								data.TimescaleInstantaneous,
								0.0,
								withdrawnRateScale,
							).Write(withdrawn/dt))
						}
					}

					if askQty > state.prevAskQty {
						replenished := askQty - state.prevAskQty
						m.SetMetric("net_replenished_quantity:ask", data.NewMetric[float64](
							"net_replenished_quantity:ask",
							data.UnitQuantity,
							data.TimescaleInstantaneous,
							0.0,
							state.prevAskQty,
						).Write(replenished))
						if state.prevAskQty > 0 {
							m.WriteNormalized("net_replenishment_fraction:ask", replenished/state.prevAskQty)
						}
						if dt > 0 {
							replenishRateScale := state.prevAskQty / dt
							m.SetMetric("net_replenishment_rate:ask", data.NewMetric[float64](
								"net_replenishment_rate:ask",
								data.UnitRate,
								data.TimescaleInstantaneous,
								0.0,
								replenishRateScale,
							).Write(replenished/dt))
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
				m.Err = errnie.Err(
					errnie.Validation,
					"toxicity: positive trade price and quantity required",
					nil,
				)

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

			var dt float64
			if state.hasPrevTime {
				dt = m.At.Sub(state.prevTime).Seconds()
				if dt > 0 {
					bidRate = state.touchFillBidQty / dt
					askRate = state.touchFillAskQty / dt
					hasRate = true
				}
			}

			state.prevTime = m.At
			state.hasPrevTime = true

			m.SetMetric("bracket_trade_quantity", data.NewMetric[float64](
				"bracket_trade_quantity",
				data.UnitQuantity,
				data.TimescaleInstantaneous,
				0.0,
				state.bracketQty,
			).Write(state.bracketQty))
			m.SetMetric("matched_touch_trade_quantity:bid", data.NewMetric[float64](
				"matched_touch_trade_quantity:bid",
				data.UnitQuantity,
				data.TimescaleInstantaneous,
				0.0,
				state.bracketQty,
			).Write(state.matchedBidQty))
			m.SetMetric("matched_touch_trade_quantity:ask", data.NewMetric[float64](
				"matched_touch_trade_quantity:ask",
				data.UnitQuantity,
				data.TimescaleInstantaneous,
				0.0,
				state.bracketQty,
			).Write(state.matchedAskQty))
			m.SetMetric("touch_fill_quantity:bid", data.NewMetric[float64](
				"touch_fill_quantity:bid",
				data.UnitQuantity,
				data.TimescaleInstantaneous,
				0.0,
				bidQty,
			).Write(state.touchFillBidQty))
			m.SetMetric("touch_fill_quantity:ask", data.NewMetric[float64](
				"touch_fill_quantity:ask",
				data.UnitQuantity,
				data.TimescaleInstantaneous,
				0.0,
				askQty,
			).Write(state.touchFillAskQty))
			m.SetMetric("touch_fill_fraction:bid", data.NewMetric[float64](
				"touch_fill_fraction:bid",
				data.UnitDimensionless,
				data.TimescaleInstantaneous,
				0.5,
				0.5,
			).Write(bidFillFrac))
			m.SetMetric("touch_fill_fraction:ask", data.NewMetric[float64](
				"touch_fill_fraction:ask",
				data.UnitDimensionless,
				data.TimescaleInstantaneous,
				0.5,
				0.5,
			).Write(askFillFrac))

			if hasRate && dt > 0 {
				m.SetMetric("touch_fill_rate:bid", data.NewMetric[float64](
					"touch_fill_rate:bid",
					data.UnitRate,
					data.TimescaleInstantaneous,
					0.0,
					bidQty/dt,
				).Write(bidRate))
				m.SetMetric("touch_fill_rate:ask", data.NewMetric[float64](
					"touch_fill_rate:ask",
					data.UnitRate,
					data.TimescaleInstantaneous,
					0.0,
					askQty/dt,
				).Write(askRate))
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
