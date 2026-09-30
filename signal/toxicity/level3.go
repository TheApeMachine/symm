package toxicity

import (
	"context"
	"fmt"
	"math"
	"time"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Level3 is the toxicity measuring instrument. It holds no state and no logic of
its own: its entire behavior is one nomagique pipeline over the measurement itself —
every stage writes its facts into the measurement where it computes them, and the
workload's register owns the measurement's lifetime.
*/
type Level3 struct {
	*runtime.System
	pipeline core.Primitive
	ID       int
}

func NewLevel3(ctx context.Context, books broker.BookSource) *Level3 {
	type level3State struct {
		hasPrev    bool
		prevBid    float64
		prevAsk    float64
		prevBidQty float64
		prevAskQty float64
		prevTime   time.Time
	}
	states := make(map[string]*level3State)

	level3 := &Level3{
		pipeline: nomagique.NewNumber(
			// 0. Extract raw depth facts and compute toxicity properties
			data.NewAdapter(
				transport.NewPass(),
				func(m *data.Measurement[float64]) *data.Measurement[float64] {
					input := m

					if len(m.Peers) > 0 {
						peer := m.FindPeer(func(p *data.Measurement[float64]) bool {
							if p.Label == "" || p.Provenance["channel"] == "ticker" || p.Provenance["channel"] == "trade" {
								return false
							}
							b := p.Metrics["best_price:bid"].Raw
							if b == 0 {
								b = p.Metrics["best_bid"].Raw
							}
							if b == 0 {
								b = p.Metrics["bid"].Raw
							}
							a := p.Metrics["best_price:ask"].Raw
							if a == 0 {
								a = p.Metrics["best_ask"].Raw
							}
							if a == 0 {
								a = p.Metrics["ask"].Raw
							}
							return b > 0 && a > 0
						})

						if peer != nil {
							input = peer
						}
					}

					bidPrice := input.Metrics["best_price:bid"].Raw
					if bidPrice == 0 {
						bidPrice = input.Metrics["best_bid"].Raw
					}
					if bidPrice == 0 {
						bidPrice = input.Metrics["bid"].Raw
					}

					askPrice := input.Metrics["best_price:ask"].Raw
					if askPrice == 0 {
						askPrice = input.Metrics["best_ask"].Raw
					}
					if askPrice == 0 {
						askPrice = input.Metrics["ask"].Raw
					}

					bidQty := input.Metrics["touch_quantity:bid"].Raw
					if bidQty == 0 {
						bidQty = input.Metrics["bid_qty"].Raw
					}

					askQty := input.Metrics["touch_quantity:ask"].Raw
					if askQty == 0 {
						askQty = input.Metrics["ask_qty"].Raw
					}

					if bidPrice <= 0 || askPrice <= 0 {
						return nil
					}

					if bidPrice >= askPrice {
						m.Err = fmt.Errorf("toxicity: crossed touch (%f >= %f)", bidPrice, askPrice)
						return m
					}

					if m.Metrics == nil {
						m.Metrics = make(map[string]data.Metric[float64])
					}
					if m.Metadata == nil {
						m.Metadata = make(map[string]string)
					}

					m.Metrics["best_price:bid"] = m.Metrics["best_price:bid"].Write(bidPrice)
					m.Metrics["best_price:ask"] = m.Metrics["best_price:ask"].Write(askPrice)
					m.Metrics["touch_quantity:bid"] = m.Metrics["touch_quantity:bid"].Write(bidQty)
					m.Metrics["touch_quantity:ask"] = m.Metrics["touch_quantity:ask"].Write(askQty)
					m.Metrics["unfilled_residual_quantity:bid"] = m.Metrics["unfilled_residual_quantity:bid"].Write(bidQty)
					m.Metrics["unfilled_residual_quantity:ask"] = m.Metrics["unfilled_residual_quantity:ask"].Write(askQty)

					state := states[m.Label]
					if state == nil {
						state = &level3State{}
						states[m.Label] = state
					}

					if state.hasPrev {
						m.Metrics["previous_best_price:bid"] = m.Metrics["previous_best_price:bid"].Write(state.prevBid)
						m.Metrics["previous_best_price:ask"] = m.Metrics["previous_best_price:ask"].Write(state.prevAsk)

						dt := input.At.Sub(state.prevTime).Seconds()

						if state.prevBid > 0 && bidPrice > 0 {
							m.Metrics["touch_price_log_change:bid"] = m.Metrics["touch_price_log_change:bid"].Write(math.Log(bidPrice / state.prevBid))
						}

						if state.prevAsk > 0 && askPrice > 0 {
							m.Metrics["touch_price_log_change:ask"] = m.Metrics["touch_price_log_change:ask"].Write(math.Log(askPrice / state.prevAsk))
						}

						if bidPrice < state.prevBid {
							m.Metrics["retreated_quantity:bid"] = m.Metrics["retreated_quantity:bid"].Write(state.prevBidQty)
							m.Metrics["retreat_fraction:bid"] = m.Metrics["retreat_fraction:bid"].Write(1.0)
							if dt > 0 {
								m.Metrics["retreat_rate:bid"] = m.Metrics["retreat_rate:bid"].Write(state.prevBidQty / dt)
							}
						}

						if bidPrice == state.prevBid {
							if bidQty < state.prevBidQty {
								withdrawn := state.prevBidQty - bidQty
								m.Metrics["net_withdrawn_quantity:bid"] = m.Metrics["net_withdrawn_quantity:bid"].Write(withdrawn)
								if state.prevBidQty > 0 {
									m.Metrics["net_withdrawal_fraction:bid"] = m.Metrics["net_withdrawal_fraction:bid"].Write(withdrawn / state.prevBidQty)
								}
								if dt > 0 {
									m.Metrics["retreat_rate:bid"] = m.Metrics["retreat_rate:bid"].Write(withdrawn / dt)
								}
							}
							if bidQty > state.prevBidQty {
								m.Metrics["net_replenished_quantity:bid"] = m.Metrics["net_replenished_quantity:bid"].Write(bidQty - state.prevBidQty)
							}
						}

						if askPrice > state.prevAsk {
							m.Metrics["retreated_quantity:ask"] = m.Metrics["retreated_quantity:ask"].Write(state.prevAskQty)
							m.Metrics["retreat_fraction:ask"] = m.Metrics["retreat_fraction:ask"].Write(1.0)
							if dt > 0 {
								m.Metrics["retreat_rate:ask"] = m.Metrics["retreat_rate:ask"].Write(state.prevAskQty / dt)
							}
						}

						if askPrice == state.prevAsk {
							if askQty < state.prevAskQty {
								withdrawn := state.prevAskQty - askQty
								m.Metrics["net_withdrawn_quantity:ask"] = m.Metrics["net_withdrawn_quantity:ask"].Write(withdrawn)
								if state.prevAskQty > 0 {
									m.Metrics["net_withdrawal_fraction:ask"] = m.Metrics["net_withdrawal_fraction:ask"].Write(withdrawn / state.prevAskQty)
								}
								if dt > 0 {
									m.Metrics["retreat_rate:ask"] = m.Metrics["retreat_rate:ask"].Write(withdrawn / dt)
								}
							}
							if askQty > state.prevAskQty {
								m.Metrics["net_replenished_quantity:ask"] = m.Metrics["net_replenished_quantity:ask"].Write(askQty - state.prevAskQty)
							}
						}
					}

					state.prevTime = input.At
					state.prevBid = bidPrice
					state.prevAsk = askPrice
					state.prevBidQty = bidQty
					state.prevAskQty = askQty
					state.hasPrev = true

					if input.Label != "" {
						m.Label = input.Label
					}
					if !input.At.IsZero() {
						m.At = input.At
					}

					return m
				},
				func(m *data.Measurement[float64], res *data.Measurement[float64]) {},
			),
			// 1. Finalize
			data.NewFinalizer[float64](),
		),
	}

	level3.System = runtime.NewSystem(ctx, "toxicity:level3", level3)
	return level3
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/
func (level3 *Level3) Step(m *data.Measurement[float64]) *data.Measurement[float64] {
	if level3.Status() != runtime.READY {
		errnie.Warn(level3.Name() + ": Step called before READY; dropping event")
		return m
	}

	if m == nil || m.Err != nil {
		return m
	}

	return data.Read[*data.Measurement[float64]](level3.pipeline.Next(
		transport.NewOne(unsafe.Pointer(&m)).Next(nil),
	))
}

/*
Register returns the measurement declaring this entity's full metric schema.
Values are empty; the workload uses this at startup to allocate the metric
schema before feeding streaming records.
*/
func (level3 *Level3) Register() *data.Measurement[float64] {
	m := data.NewMeasurement("toxicity:level3", map[string]data.Metric[float64]{
		"best_price:bid":                 data.NewMetric[float64]("best_price:bid", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"best_price:ask":                 data.NewMetric[float64]("best_price:ask", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"touch_quantity:bid":             data.NewMetric[float64]("touch_quantity:bid", data.UnitCount, data.TimescaleInstantaneous, 0, 0),
		"touch_quantity:ask":             data.NewMetric[float64]("touch_quantity:ask", data.UnitCount, data.TimescaleInstantaneous, 0, 0),
		"unfilled_residual_quantity:bid": data.NewMetric[float64]("unfilled_residual_quantity:bid", data.UnitCount, data.TimescaleInstantaneous, 0, 0),
		"unfilled_residual_quantity:ask": data.NewMetric[float64]("unfilled_residual_quantity:ask", data.UnitCount, data.TimescaleInstantaneous, 0, 0),
		"previous_best_price:bid":        data.NewMetric[float64]("previous_best_price:bid", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"previous_best_price:ask":        data.NewMetric[float64]("previous_best_price:ask", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"touch_price_log_change:bid":     data.NewMetric[float64]("touch_price_log_change:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 0),
		"touch_price_log_change:ask":     data.NewMetric[float64]("touch_price_log_change:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 0),
		"retreated_quantity:bid":         data.NewMetric[float64]("retreated_quantity:bid", data.UnitCount, data.TimescaleInstantaneous, 0, 0),
		"net_withdrawn_quantity:bid":     data.NewMetric[float64]("net_withdrawn_quantity:bid", data.UnitCount, data.TimescaleInstantaneous, 0, 0),
		"net_replenished_quantity:bid":   data.NewMetric[float64]("net_replenished_quantity:bid", data.UnitCount, data.TimescaleInstantaneous, 0, 0),
		"retreat_fraction:bid":           data.NewMetric[float64]("retreat_fraction:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 0),
		"net_withdrawal_fraction:bid":    data.NewMetric[float64]("net_withdrawal_fraction:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 0),
		"retreat_rate:bid":               data.NewMetric[float64]("retreat_rate:bid", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"retreated_quantity:ask":         data.NewMetric[float64]("retreated_quantity:ask", data.UnitCount, data.TimescaleInstantaneous, 0, 0),
		"net_withdrawn_quantity:ask":     data.NewMetric[float64]("net_withdrawn_quantity:ask", data.UnitCount, data.TimescaleInstantaneous, 0, 0),
		"net_replenished_quantity:ask":   data.NewMetric[float64]("net_replenished_quantity:ask", data.UnitCount, data.TimescaleInstantaneous, 0, 0),
		"retreat_fraction:ask":           data.NewMetric[float64]("retreat_fraction:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 0),
		"net_withdrawal_fraction:ask":    data.NewMetric[float64]("net_withdrawal_fraction:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 0),
		"retreat_rate:ask":               data.NewMetric[float64]("retreat_rate:ask", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
	})
	m.Metadata["peer-interest"] = "*"
	return m
}
