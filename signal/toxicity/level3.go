package toxicity

import (
	"context"
	"fmt"
	"math"
	"sync"
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
	pipelines sync.Map
	ID        int
	books     broker.BookSource
}

func NewLevel3(ctx context.Context, books broker.BookSource) *Level3 {
	level3 := &Level3{
		books: books,
	}

	level3.System = runtime.NewSystem(ctx, "toxicity:level3", level3)
	return level3
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/

func (level3 *Level3) pipelineFor(symbol string) core.Primitive {
	if existing, ok := level3.pipelines.Load(symbol); ok {
		return existing.(core.Primitive)
	}

	type level3State struct {
		hasPrev    bool
		prevBid    float64
		prevAsk    float64
		prevBidQty float64
		prevAskQty float64
		prevTime   time.Time
	}
	state := &level3State{}

	pipeline := nomagique.NewNumber(
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
						b := p.GetMetric("best_price:bid").Raw
						if b == 0 {
							b = p.GetMetric("best_bid").Raw
						}
						if b == 0 {
							b = p.GetMetric("bid").Raw
						}
						a := p.GetMetric("best_price:ask").Raw
						if a == 0 {
							a = p.GetMetric("best_ask").Raw
						}
						if a == 0 {
							a = p.GetMetric("ask").Raw
						}
						return b > 0 && a > 0
					})

					if peer != nil {
						input = peer
					}
				}

				bidPrice := input.GetMetric("best_price:bid").Raw
				if bidPrice == 0 {
					bidPrice = input.GetMetric("best_bid").Raw
				}
				if bidPrice == 0 {
					bidPrice = input.GetMetric("bid").Raw
				}

				askPrice := input.GetMetric("best_price:ask").Raw
				if askPrice == 0 {
					askPrice = input.GetMetric("best_ask").Raw
				}
				if askPrice == 0 {
					askPrice = input.GetMetric("ask").Raw
				}

				bidQty := input.GetMetric("touch_quantity:bid").Raw
				if bidQty == 0 {
					bidQty = input.GetMetric("bid_qty").Raw
				}

				askQty := input.GetMetric("touch_quantity:ask").Raw
				if askQty == 0 {
					askQty = input.GetMetric("ask_qty").Raw
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
				m.EnsureMetadata()

				m.WriteMetric("best_price:bid", bidPrice)
				m.WriteMetric("best_price:ask", askPrice)
				m.WriteMetric("touch_quantity:bid", bidQty)
				m.WriteMetric("touch_quantity:ask", askQty)
				m.WriteMetric("unfilled_residual_quantity:bid", bidQty)
				m.WriteMetric("unfilled_residual_quantity:ask", askQty)


				if state.hasPrev {
					m.WriteMetric("previous_best_price:bid", state.prevBid)
					m.WriteMetric("previous_best_price:ask", state.prevAsk)

					dt := input.At.Sub(state.prevTime).Seconds()

					if state.prevBid > 0 && bidPrice > 0 {
						m.WriteMetric("touch_price_log_change:bid", math.Log(bidPrice / state.prevBid))
					}

					if state.prevAsk > 0 && askPrice > 0 {
						m.WriteMetric("touch_price_log_change:ask", math.Log(askPrice / state.prevAsk))
					}

					if bidPrice < state.prevBid {
						m.WriteMetric("retreated_quantity:bid", state.prevBidQty)
						m.WriteMetric("retreat_fraction:bid", 1.0)
						if dt > 0 {
							m.WriteMetric("retreat_rate:bid", state.prevBidQty / dt)
						}
					}

					if bidPrice == state.prevBid {
						if bidQty < state.prevBidQty {
							withdrawn := state.prevBidQty - bidQty
							m.WriteMetric("net_withdrawn_quantity:bid", withdrawn)
							if state.prevBidQty > 0 {
								m.WriteMetric("net_withdrawal_fraction:bid", withdrawn / state.prevBidQty)
							}
							if dt > 0 {
								m.WriteMetric("retreat_rate:bid", withdrawn / dt)
							}
						}
						if bidQty > state.prevBidQty {
							m.WriteMetric("net_replenished_quantity:bid", bidQty - state.prevBidQty)
						}
					}

					if askPrice > state.prevAsk {
						m.WriteMetric("retreated_quantity:ask", state.prevAskQty)
						m.WriteMetric("retreat_fraction:ask", 1.0)
						if dt > 0 {
							m.WriteMetric("retreat_rate:ask", state.prevAskQty / dt)
						}
					}

					if askPrice == state.prevAsk {
						if askQty < state.prevAskQty {
							withdrawn := state.prevAskQty - askQty
							m.WriteMetric("net_withdrawn_quantity:ask", withdrawn)
							if state.prevAskQty > 0 {
								m.WriteMetric("net_withdrawal_fraction:ask", withdrawn / state.prevAskQty)
							}
							if dt > 0 {
								m.WriteMetric("retreat_rate:ask", withdrawn / dt)
							}
						}
						if askQty > state.prevAskQty {
							m.WriteMetric("net_replenished_quantity:ask", askQty - state.prevAskQty)
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
	)

	actual, _ := level3.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

func (level3 *Level3) Step(m *data.Measurement[float64]) *data.Measurement[float64] {
	if level3.Status() != runtime.READY {
		errnie.Warn(level3.Name() + ": Step called before READY; dropping event")
		return m
	}

	if m == nil || m.Err != nil {
		return m
	}

	return data.Read[*data.Measurement[float64]](level3.pipelineFor(m.Label).Next(
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
	m.SetMetadata("peer-interest", "*")
	return m
}
