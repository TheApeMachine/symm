package toxicity

import (
	"context"
	"fmt"
	"github.com/theapemachine/errnie"
	"iter"
	"math"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

type level3Input struct {
	Symbol   string
	BidPrice float64
	AskPrice float64
	BidQty   float64
	AskQty   float64
	At       time.Time
}

type level3Result struct {
	BidPrice             float64
	AskPrice             float64
	BidQty               float64
	AskQty               float64
	HasPrev              bool
	PrevBid              float64
	PrevAsk              float64
	BidLogChange         float64
	AskLogChange         float64
	HasBidLogChange      bool
	HasAskLogChange      bool
	RetreatedBidQty      float64
	RetreatedAskQty      float64
	NetWithdrawnBidQty   float64
	NetWithdrawnAskQty   float64
	NetReplenishedBidQty float64
	NetReplenishedAskQty float64
	RetreatBidFraction   float64
	RetreatAskFraction   float64
	NetWithdrawBidFrac   float64
	NetWithdrawAskFrac   float64
	RetreatBidRate       float64
	RetreatAskRate       float64
	NetWithdrawBidRate   float64
	NetWithdrawAskRate   float64
	HasRates             bool
}

type level3State struct {
	hasPrev    bool
	prevBid    float64
	prevAsk    float64
	prevBidQty float64
	prevAskQty float64
	prevTime   time.Time
}

type level3Pipeline struct {
	*core.PrimitiveError
	paths map[string]*level3State
	out   level3Result
}

func newLevel3Pipeline() core.Primitive {
	return &level3Pipeline{
		PrimitiveError: core.NewPrimitiveError(),
		paths:          make(map[string]*level3State),
	}
}

func (op *level3Pipeline) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*level3Input)(arriving)

			state := op.paths[input.Symbol]
			if state == nil {
				state = &level3State{}
				op.paths[input.Symbol] = state
			}

			op.out = level3Result{
				BidPrice: input.BidPrice,
				AskPrice: input.AskPrice,
				BidQty:   input.BidQty,
				AskQty:   input.AskQty,
			}

			if state.hasPrev {
				op.out.HasPrev = true
				op.out.PrevBid = state.prevBid
				op.out.PrevAsk = state.prevAsk

				dt := input.At.Sub(state.prevTime).Seconds()

				if state.prevBid > 0 && input.BidPrice > 0 {
					op.out.BidLogChange = math.Log(input.BidPrice / state.prevBid)
					op.out.HasBidLogChange = true
				}

				if state.prevAsk > 0 && input.AskPrice > 0 {
					op.out.AskLogChange = math.Log(input.AskPrice / state.prevAsk)
					op.out.HasAskLogChange = true
				}

				if input.BidPrice < state.prevBid {
					op.out.RetreatedBidQty = state.prevBidQty
					op.out.RetreatBidFraction = 1.0
					if dt > 0 {
						op.out.RetreatBidRate = state.prevBidQty / dt
						op.out.HasRates = true
					}
				}

				if input.BidPrice == state.prevBid {
					if input.BidQty < state.prevBidQty {
						withdrawn := state.prevBidQty - input.BidQty
						op.out.NetWithdrawnBidQty = withdrawn
						if state.prevBidQty > 0 {
							op.out.NetWithdrawBidFrac = withdrawn / state.prevBidQty
						}
						if dt > 0 {
							op.out.NetWithdrawBidRate = withdrawn / dt
							op.out.HasRates = true
						}
					}
					if input.BidQty > state.prevBidQty {
						op.out.NetReplenishedBidQty = input.BidQty - state.prevBidQty
					}
				}

				if input.AskPrice > state.prevAsk {
					op.out.RetreatedAskQty = state.prevAskQty
					op.out.RetreatAskFraction = 1.0
					if dt > 0 {
						op.out.RetreatAskRate = state.prevAskQty / dt
						op.out.HasRates = true
					}
				}

				if input.AskPrice == state.prevAsk {
					if input.AskQty < state.prevAskQty {
						withdrawn := state.prevAskQty - input.AskQty
						op.out.NetWithdrawnAskQty = withdrawn
						if state.prevAskQty > 0 {
							op.out.NetWithdrawAskFrac = withdrawn / state.prevAskQty
						}
						if dt > 0 {
							op.out.NetWithdrawAskRate = withdrawn / dt
							op.out.HasRates = true
						}
					}
					if input.AskQty > state.prevAskQty {
						op.out.NetReplenishedAskQty = input.AskQty - state.prevAskQty
					}
				}
			}

			state.prevBid = input.BidPrice
			state.prevAsk = input.AskPrice
			state.prevBidQty = input.BidQty
			state.prevAskQty = input.AskQty
			state.prevTime = input.At
			state.hasPrev = true

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Level3 is the book-touch market entity. It holds no state and no logic of its own:
its entire behavior is one nomagique pipeline over the measurement itself —
every stage writes its facts into the measurement where it computes them, and the
workload's register owns the measurement's lifetime.
*/
type Level3 struct {
	*runtime.System
	pipeline core.Primitive
	ID       int
}

func NewLevel3(ctx context.Context) *Level3 {
	level3 := &Level3{
		pipeline: nomagique.NewNumber(newLevel3Pipeline()),
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

	if m == nil {
		return nil
	}

	if m.Err != nil {
		return m
	}

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

		if peer == nil {
			return nil
		}

		input = peer
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
		return m
	}

	if bidPrice >= askPrice {
		m.Err = fmt.Errorf("toxicity: crossed touch (%f >= %f)", bidPrice, askPrice)
		return m
	}

	if m.Metadata == nil {
		m.Metadata = make(map[string]string)
	}

	pipeInput := level3Input{
		Symbol:   input.Label,
		BidPrice: bidPrice,
		AskPrice: askPrice,
		BidQty:   bidQty,
		AskQty:   askQty,
		At:       input.At,
	}

	for out := range level3.pipeline.Next(transport.NewOne(unsafe.Pointer(&pipeInput)).Next(nil)) {
		res := (*level3Result)(out)

		m.Metrics["best_price:bid"] = m.Metrics["best_price:bid"].Write(res.BidPrice)
		m.Metrics["best_price:ask"] = m.Metrics["best_price:ask"].Write(res.AskPrice)
		m.Metrics["touch_quantity:bid"] = m.Metrics["touch_quantity:bid"].Write(res.BidQty)
		m.Metrics["touch_quantity:ask"] = m.Metrics["touch_quantity:ask"].Write(res.AskQty)
		m.Metrics["unfilled_residual_quantity:bid"] = m.Metrics["unfilled_residual_quantity:bid"].Write(res.BidQty)
		m.Metrics["unfilled_residual_quantity:ask"] = m.Metrics["unfilled_residual_quantity:ask"].Write(res.AskQty)

		if res.HasPrev {
			m.Metrics["previous_best_price:bid"] = m.Metrics["previous_best_price:bid"].Write(res.PrevBid)
			m.Metrics["previous_best_price:ask"] = m.Metrics["previous_best_price:ask"].Write(res.PrevAsk)
		}

		if res.HasBidLogChange {
			m.Metrics["touch_price_log_change:bid"] = m.Metrics["touch_price_log_change:bid"].Write(res.BidLogChange)
		}

		if res.HasAskLogChange {
			m.Metrics["touch_price_log_change:ask"] = m.Metrics["touch_price_log_change:ask"].Write(res.AskLogChange)
		}

		m.Metrics["retreated_quantity:bid"] = m.Metrics["retreated_quantity:bid"].Write(res.RetreatedBidQty)
		m.Metrics["net_withdrawn_quantity:bid"] = m.Metrics["net_withdrawn_quantity:bid"].Write(res.NetWithdrawnBidQty)
		m.Metrics["net_replenished_quantity:bid"] = m.Metrics["net_replenished_quantity:bid"].Write(res.NetReplenishedBidQty)
		m.Metrics["retreat_fraction:bid"] = m.Metrics["retreat_fraction:bid"].Write(res.RetreatBidFraction)
		m.Metrics["net_withdrawal_fraction:bid"] = m.Metrics["net_withdrawal_fraction:bid"].Write(res.NetWithdrawBidFrac)
		m.Metrics["retreat_rate:bid"] = m.Metrics["retreat_rate:bid"].Write(res.RetreatBidRate)

		m.Metrics["retreated_quantity:ask"] = m.Metrics["retreated_quantity:ask"].Write(res.RetreatedAskQty)
		m.Metrics["net_withdrawn_quantity:ask"] = m.Metrics["net_withdrawn_quantity:ask"].Write(res.NetWithdrawnAskQty)
		m.Metrics["net_replenished_quantity:ask"] = m.Metrics["net_replenished_quantity:ask"].Write(res.NetReplenishedAskQty)
		m.Metrics["retreat_fraction:ask"] = m.Metrics["retreat_fraction:ask"].Write(res.RetreatAskFraction)
		m.Metrics["net_withdrawal_fraction:ask"] = m.Metrics["net_withdrawal_fraction:ask"].Write(res.NetWithdrawAskFrac)
		m.Metrics["retreat_rate:ask"] = m.Metrics["retreat_rate:ask"].Write(res.RetreatAskRate)
	}

	m.Label = input.Label
	m.At = input.At
	m.Finalize()
	return m
}

/*
Register returns the measurement declaring this entity's full metric schema.
Values are empty; the workload uses this at startup to allocate the metric
schema before feeding streaming records.
*/
func (level3 *Level3) Register() *data.Measurement[float64] {
	m := data.NewMeasurement("toxicity:level3", map[string]data.Metric[float64]{
		"best_price:bid":                 data.NewMetric[float64]("best_price:bid", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"best_price:ask":                 data.NewMetric[float64]("best_price:ask", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"touch_quantity:bid":             data.NewMetric[float64]("touch_quantity:bid", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"touch_quantity:ask":             data.NewMetric[float64]("touch_quantity:ask", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"unfilled_residual_quantity:bid": data.NewMetric[float64]("unfilled_residual_quantity:bid", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"unfilled_residual_quantity:ask": data.NewMetric[float64]("unfilled_residual_quantity:ask", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"previous_best_price:bid":        data.NewMetric[float64]("previous_best_price:bid", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"previous_best_price:ask":        data.NewMetric[float64]("previous_best_price:ask", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"touch_price_log_change:bid":     data.NewMetric[float64]("touch_price_log_change:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"touch_price_log_change:ask":     data.NewMetric[float64]("touch_price_log_change:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"retreated_quantity:bid":         data.NewMetric[float64]("retreated_quantity:bid", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"net_withdrawn_quantity:bid":     data.NewMetric[float64]("net_withdrawn_quantity:bid", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"net_replenished_quantity:bid":   data.NewMetric[float64]("net_replenished_quantity:bid", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"retreat_fraction:bid":           data.NewMetric[float64]("retreat_fraction:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"net_withdrawal_fraction:bid":    data.NewMetric[float64]("net_withdrawal_fraction:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"retreat_rate:bid":               data.NewMetric[float64]("retreat_rate:bid", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"retreated_quantity:ask":         data.NewMetric[float64]("retreated_quantity:ask", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"net_withdrawn_quantity:ask":     data.NewMetric[float64]("net_withdrawn_quantity:ask", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"net_replenished_quantity:ask":   data.NewMetric[float64]("net_replenished_quantity:ask", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"retreat_fraction:ask":           data.NewMetric[float64]("retreat_fraction:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"net_withdrawal_fraction:ask":    data.NewMetric[float64]("net_withdrawal_fraction:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"retreat_rate:ask":               data.NewMetric[float64]("retreat_rate:ask", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
	})
	m.Metadata["peer-interest"] = "*"
	return m
}
