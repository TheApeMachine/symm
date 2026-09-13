package toxicity

import (
	"context"
	"fmt"
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

type level3Pipeline struct {
	*core.PrimitiveError
	hasPrev    bool
	prevBid    float64
	prevAsk    float64
	prevBidQty float64
	prevAskQty float64
	prevTime   time.Time
	out        level3Result
}

func newLevel3Pipeline() core.Primitive {
	return &level3Pipeline{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *level3Pipeline) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*level3Input)(arriving)

			op.out = level3Result{
				BidPrice: input.BidPrice,
				AskPrice: input.AskPrice,
				BidQty:   input.BidQty,
				AskQty:   input.AskQty,
			}

			if op.hasPrev {
				op.out.HasPrev = true
				op.out.PrevBid = op.prevBid
				op.out.PrevAsk = op.prevAsk

				dt := input.At.Sub(op.prevTime).Seconds()

				if op.prevBid > 0 && input.BidPrice > 0 {
					op.out.BidLogChange = math.Log(input.BidPrice / op.prevBid)
					op.out.HasBidLogChange = true
				}

				if op.prevAsk > 0 && input.AskPrice > 0 {
					op.out.AskLogChange = math.Log(input.AskPrice / op.prevAsk)
					op.out.HasAskLogChange = true
				}

				if input.BidPrice < op.prevBid {
					op.out.RetreatedBidQty = op.prevBidQty
					op.out.RetreatBidFraction = 1.0
					if dt > 0 {
						op.out.RetreatBidRate = op.prevBidQty / dt
						op.out.HasRates = true
					}
				}

				if input.BidPrice == op.prevBid {
					if input.BidQty < op.prevBidQty {
						withdrawn := op.prevBidQty - input.BidQty
						op.out.NetWithdrawnBidQty = withdrawn
						if op.prevBidQty > 0 {
							op.out.NetWithdrawBidFrac = withdrawn / op.prevBidQty
						}
						if dt > 0 {
							op.out.NetWithdrawBidRate = withdrawn / dt
							op.out.HasRates = true
						}
					}
					if input.BidQty > op.prevBidQty {
						op.out.NetReplenishedBidQty = input.BidQty - op.prevBidQty
					}
				}

				if input.AskPrice > op.prevAsk {
					op.out.RetreatedAskQty = op.prevAskQty
					op.out.RetreatAskFraction = 1.0
					if dt > 0 {
						op.out.RetreatAskRate = op.prevAskQty / dt
						op.out.HasRates = true
					}
				}

				if input.AskPrice == op.prevAsk {
					if input.AskQty < op.prevAskQty {
						withdrawn := op.prevAskQty - input.AskQty
						op.out.NetWithdrawnAskQty = withdrawn
						if op.prevAskQty > 0 {
							op.out.NetWithdrawAskFrac = withdrawn / op.prevAskQty
						}
						if dt > 0 {
							op.out.NetWithdrawAskRate = withdrawn / dt
							op.out.HasRates = true
						}
					}
					if input.AskQty > op.prevAskQty {
						op.out.NetReplenishedAskQty = input.AskQty - op.prevAskQty
					}
				}
			}

			op.prevBid = input.BidPrice
			op.prevAsk = input.AskPrice
			op.prevBidQty = input.BidQty
			op.prevAskQty = input.AskQty
			op.prevTime = input.At
			op.hasPrev = true

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
	return &Level3{
		System:   runtime.NewSystem(ctx, "toxicity:level3"),
		pipeline: nomagique.NewNumber(newLevel3Pipeline()),
	}
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/
func (level3 *Level3) Step(m *data.Measurement[float64]) *data.Measurement[float64] {
	if m == nil {
		return nil
	}

	if m.Err != nil {
		return m
	}

	bidPrice := m.Metrics["best_price:bid"].Raw
	askPrice := m.Metrics["best_price:ask"].Raw
	bidQty := m.Metrics["touch_quantity:bid"].Raw
	askQty := m.Metrics["touch_quantity:ask"].Raw

	if bidPrice <= 0 || askPrice <= 0 {
		return m
	}

	if bidPrice >= askPrice {
		m.Err = fmt.Errorf("toxicity: crossed touch (%f >= %f)", bidPrice, askPrice)
		return m
	}

	if m.Metadata == nil {
		m.Metadata = make(map[string]float64)
	}

	input := level3Input{
		BidPrice: bidPrice,
		AskPrice: askPrice,
		BidQty:   bidQty,
		AskQty:   askQty,
		At:       m.At,
	}

	for out := range level3.pipeline.Next(transport.NewOne(unsafe.Pointer(&input)).Next(nil)) {
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

	m.Finalize()
	return m
}

/*
Register returns the measurement declaring this entity's full metric schema.
Values are empty; the workload uses this at startup to allocate the metric
schema before feeding streaming records.
*/
func (level3 *Level3) Register() *data.Measurement[float64] {
	return data.NewMeasurement[float64]("toxicity:level3", map[string]data.Metric[float64]{
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
}
