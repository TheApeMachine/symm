package pumpdump

import (
	"context"
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

type level3EntityInput struct {
	Bid float64
	Ask float64
}

type level3EntityResult struct {
	Bid            float64
	Ask            float64
	Midpoint       float64
	Spread         float64
	RelativeSpread float64
	Valid          bool
}

type level3EntityPipeline struct {
	*core.PrimitiveError
	hasBid  bool
	hasAsk  bool
	prevBid float64
	prevAsk float64
	out     level3EntityResult
}

func newLevel3EntityPipeline() core.Primitive {
	return &level3EntityPipeline{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *level3EntityPipeline) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*level3EntityInput)(arriving)

			if input.Bid > 0 {
				op.prevBid = input.Bid
				op.hasBid = true
			}

			if input.Ask > 0 {
				op.prevAsk = input.Ask
				op.hasAsk = true
			}

			if !op.hasBid || !op.hasAsk {
				op.out = level3EntityResult{Valid: false}
				if !yield(unsafe.Pointer(&op.out)) {
					return
				}
				continue
			}

			midpoint := (op.prevBid + op.prevAsk) / 2.0
			spread := op.prevAsk - op.prevBid
			relativeSpread := 0.0

			if midpoint > 0 {
				relativeSpread = spread / midpoint
			}

			op.out = level3EntityResult{
				Bid:            op.prevBid,
				Ask:            op.prevAsk,
				Midpoint:       midpoint,
				Spread:         spread,
				RelativeSpread: relativeSpread,
				Valid:          true,
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Level3 is the authoritative executable-touch market entity. It holds no state
and no logic of its own: its entire behavior is one nomagique pipeline over the
measurement itself — every stage writes its facts into the measurement where it
computes them, and the workload's register owns the measurement's lifetime.
*/
type Level3 struct {
	*runtime.System
	pipeline core.Primitive
	ID       int
}

func NewLevel3(ctx context.Context) *Level3 {
	return &Level3{
		System:   runtime.NewSystem(ctx, "pumpdump:level3"),
		pipeline: nomagique.NewNumber(newLevel3EntityPipeline()),
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

	bid := m.Metrics["best_bid"].Raw
	ask := m.Metrics["best_ask"].Raw

	if bid > 0 && ask > 0 && bid >= ask {
		m.Err = fmt.Errorf("pumpdump: crossed touch (%f >= %f)", bid, ask)
		return m
	}

	if m.Metadata == nil {
		m.Metadata = make(map[string]float64)
	}

	input := level3EntityInput{Bid: bid, Ask: ask}

	var valid bool
	for out := range level3.pipeline.Next(transport.NewOne(unsafe.Pointer(&input)).Next(nil)) {
		res := (*level3EntityResult)(out)
		if !res.Valid {
			return nil
		}

		valid = true
		m.Metrics["best_bid"] = m.Metrics["best_bid"].Write(res.Bid)
		m.Metrics["best_ask"] = m.Metrics["best_ask"].Write(res.Ask)
		m.Metrics["midpoint"] = m.Metrics["midpoint"].Write(res.Midpoint)
		m.Metrics["spread"] = m.Metrics["spread"].Write(res.Spread)
		m.Metrics["relative_spread"] = m.Metrics["relative_spread"].Write(res.RelativeSpread)
		m.Maturity = 1.0
	}

	if !valid {
		return nil
	}

	return m
}

/*
Register returns the measurement declaring this entity's full metric schema.
Values are empty; the workload uses this at startup to allocate the metric
schema before feeding streaming records.
*/
func (level3 *Level3) Register() *data.Measurement[float64] {
	return data.NewMeasurement[float64]("pumpdump:level3", map[string]data.Metric[float64]{
		"best_bid":        data.NewMetric[float64]("best_bid", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"best_ask":        data.NewMetric[float64]("best_ask", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"midpoint":        data.NewMetric[float64]("midpoint", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"spread":          data.NewMetric[float64]("spread", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"relative_spread": data.NewMetric[float64]("relative_spread", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
	})
}
