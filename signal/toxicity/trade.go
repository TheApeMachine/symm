package toxicity

import (
	"context"
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

type tradeInput struct {
	Price    float64
	Qty      float64
	Side     string
	BidPrice float64
	AskPrice float64
	BidQty   float64
	AskQty   float64
	At       time.Time
}

type tradeResult struct {
	BracketQty       float64
	MatchedBidQty    float64
	MatchedAskQty    float64
	TouchFillBidQty  float64
	TouchFillAskQty  float64
	TouchFillBidFrac float64
	TouchFillAskFrac float64
	TouchFillBidRate float64
	TouchFillAskRate float64
	HasRate          bool
	BidReading       adaptive.BaselineReading
	AskReading       adaptive.BaselineReading
	BidSupported     bool
	AskSupported     bool
}

type tradePipeline struct {
	*core.PrimitiveError
	bracketQty         float64
	matchedBidQty      float64
	matchedAskQty      float64
	touchFillBidQty    float64
	touchFillAskQty    float64
	hasPrevTime        bool
	prevTime           time.Time
	bidBaseline        core.Primitive
	askBaseline        core.Primitive
	bidFractionSamples int
	askFractionSamples int
	out                tradeResult
}

func newTradePipeline() core.Primitive {
	return &tradePipeline{
		PrimitiveError: core.NewPrimitiveError(),
		bidBaseline:    adaptive.NewBaseline(adaptive.NewWindow()),
		askBaseline:    adaptive.NewBaseline(adaptive.NewWindow()),
	}
}

func (op *tradePipeline) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*tradeInput)(arriving)

			if input.Price <= 0 || input.Qty <= 0 {
				continue
			}

			inBracket := (input.Price >= input.BidPrice && input.Price <= input.AskPrice)
			if inBracket {
				op.bracketQty += input.Qty
			}

			var bidFillFrac, askFillFrac float64

			if input.Side == "sell" && input.Price == input.BidPrice {
				op.matchedBidQty += input.Qty
				op.touchFillBidQty += input.Qty
				if input.BidQty > 0 {
					bidFillFrac = op.touchFillBidQty / input.BidQty
				}
			}

			if input.Side == "buy" && input.Price == input.AskPrice {
				op.matchedAskQty += input.Qty
				op.touchFillAskQty += input.Qty
				if input.AskQty > 0 {
					askFillFrac = op.touchFillAskQty / input.AskQty
				}
			}

			var bidRate, askRate float64
			var hasRate bool

			if op.hasPrevTime {
				dt := input.At.Sub(op.prevTime).Seconds()
				if dt > 0 {
					bidRate = op.touchFillBidQty / dt
					askRate = op.touchFillAskQty / dt
					hasRate = true
				}
			}

			op.prevTime = input.At
			op.hasPrevTime = true

			var bidReading, askReading adaptive.BaselineReading

			if bidFillFrac > 0 {
				for rPtr := range op.bidBaseline.Next(transport.NewOne(unsafe.Pointer(&bidFillFrac)).Next(nil)) {
					bidReading = *(*adaptive.BaselineReading)(rPtr)
				}
				op.bidFractionSamples++
			}

			if askFillFrac > 0 {
				for rPtr := range op.askBaseline.Next(transport.NewOne(unsafe.Pointer(&askFillFrac)).Next(nil)) {
					askReading = *(*adaptive.BaselineReading)(rPtr)
				}
				op.askFractionSamples++
			}

			op.out = tradeResult{
				BracketQty:       op.bracketQty,
				MatchedBidQty:    op.matchedBidQty,
				MatchedAskQty:    op.matchedAskQty,
				TouchFillBidQty:  op.touchFillBidQty,
				TouchFillAskQty:  op.touchFillAskQty,
				TouchFillBidFrac: bidFillFrac,
				TouchFillAskFrac: askFillFrac,
				TouchFillBidRate: bidRate,
				TouchFillAskRate: askRate,
				HasRate:          hasRate,
				BidReading:       bidReading,
				AskReading:       askReading,
				BidSupported:     op.bidFractionSamples >= 3,
				AskSupported:     op.askFractionSamples >= 3,
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Trade matches incoming trades against the symbol's book touch. It holds no
state and no logic of its own: its entire behavior is one nomagique pipeline
over the measurement itself — every stage writes its facts into the measurement
where it computes them, and the workload's register owns the measurement's lifetime.
*/
type Trade struct {
	*runtime.System
	pipeline core.Primitive
	ID       int
}

func NewTrade(ctx context.Context) *Trade {
	return &Trade{
		System:   runtime.NewSystem(ctx, "toxicity:trade"),
		pipeline: nomagique.NewNumber(newTradePipeline()),
	}
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/
func (trade *Trade) Step(m *data.Measurement[float64]) *data.Measurement[float64] {
	if m == nil {
		return nil
	}

	if m.Err != nil {
		return m
	}

	if m.Metadata == nil {
		m.Metadata = make(map[string]float64)
	}

	input := tradeInput{
		Price:    m.Metrics["price"].Raw,
		Qty:      m.Metrics["qty"].Raw,
		Side:     m.Provenance["side"],
		BidPrice: m.Metrics["best_price:bid"].Raw,
		AskPrice: m.Metrics["best_price:ask"].Raw,
		BidQty:   m.Metrics["touch_quantity:bid"].Raw,
		AskQty:   m.Metrics["touch_quantity:ask"].Raw,
		At:       m.At,
	}

	for out := range trade.pipeline.Next(transport.NewOne(unsafe.Pointer(&input)).Next(nil)) {
		res := (*tradeResult)(out)

		m.Metrics["bracket_trade_quantity"] = m.Metrics["bracket_trade_quantity"].Write(res.BracketQty)
		m.Metrics["matched_touch_trade_quantity:bid"] = m.Metrics["matched_touch_trade_quantity:bid"].Write(res.MatchedBidQty)
		m.Metrics["matched_touch_trade_quantity:ask"] = m.Metrics["matched_touch_trade_quantity:ask"].Write(res.MatchedAskQty)
		m.Metrics["touch_fill_quantity:bid"] = m.Metrics["touch_fill_quantity:bid"].Write(res.TouchFillBidQty)
		m.Metrics["touch_fill_quantity:ask"] = m.Metrics["touch_fill_quantity:ask"].Write(res.TouchFillAskQty)
		m.Metrics["touch_fill_fraction:bid"] = m.Metrics["touch_fill_fraction:bid"].Write(res.TouchFillBidFrac)
		m.Metrics["touch_fill_fraction:ask"] = m.Metrics["touch_fill_fraction:ask"].Write(res.TouchFillAskFrac)

		if res.HasRate {
			m.Metrics["touch_fill_rate:bid"] = m.Metrics["touch_fill_rate:bid"].Write(res.TouchFillBidRate)
			m.Metrics["touch_fill_rate:ask"] = m.Metrics["touch_fill_rate:ask"].Write(res.TouchFillAskRate)
		}

		if res.TouchFillBidFrac > 0 && res.BidReading.HasPrior {
			m.Metrics["fill_fraction_baseline:bid"] = m.Metrics["fill_fraction_baseline:bid"].Write(res.BidReading.Baseline)
			m.Metrics["fill_fraction_divergence:bid"] = m.Metrics["fill_fraction_divergence:bid"].Write(res.BidReading.Residual)
			m.Metrics["fill_fraction_zscore:bid"] = m.Metrics["fill_fraction_zscore:bid"].Write(res.BidReading.ZScore)
		}

		if res.TouchFillAskFrac > 0 && res.AskReading.HasPrior {
			m.Metrics["fill_fraction_baseline:ask"] = m.Metrics["fill_fraction_baseline:ask"].Write(res.AskReading.Baseline)
			m.Metrics["fill_fraction_divergence:ask"] = m.Metrics["fill_fraction_divergence:ask"].Write(res.AskReading.Residual)
			m.Metrics["fill_fraction_zscore:ask"] = m.Metrics["fill_fraction_zscore:ask"].Write(res.AskReading.ZScore)
		}

		if res.BidSupported {
			m.Metadata[data.MetadataSupport] = res.BidReading.Count
			m.Metadata[data.MetadataDivergence] = res.BidReading.Residual
			if res.BidReading.VarianceDefined {
				m.Metadata[data.MetadataNoiseVariance] = res.BidReading.Variance
			}
		}

		if res.AskSupported {
			m.Metadata[data.MetadataSupport] = res.AskReading.Count
			m.Metadata[data.MetadataDivergence] = res.AskReading.Residual
			if res.AskReading.VarianceDefined {
				m.Metadata[data.MetadataNoiseVariance] = res.AskReading.Variance
			}
		}
	}

	m.Finalize()
	return m
}

/*
Register returns the measurement declaring this entity's full metric schema.
Values are empty; the workload uses this at startup to allocate the metric
schema before feeding streaming records.
*/
func (trade *Trade) Register() *data.Measurement[float64] {
	return data.NewMeasurement[float64]("toxicity:trade", map[string]data.Metric[float64]{
		"bracket_trade_quantity":             data.NewMetric[float64]("bracket_trade_quantity", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"matched_touch_trade_quantity:bid":   data.NewMetric[float64]("matched_touch_trade_quantity:bid", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"matched_touch_trade_quantity:ask":   data.NewMetric[float64]("matched_touch_trade_quantity:ask", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"touch_fill_quantity:bid":            data.NewMetric[float64]("touch_fill_quantity:bid", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"touch_fill_quantity:ask":            data.NewMetric[float64]("touch_fill_quantity:ask", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"touch_fill_fraction:bid":            data.NewMetric[float64]("touch_fill_fraction:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"touch_fill_fraction:ask":            data.NewMetric[float64]("touch_fill_fraction:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"touch_fill_rate:bid":                data.NewMetric[float64]("touch_fill_rate:bid", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"touch_fill_rate:ask":                data.NewMetric[float64]("touch_fill_rate:ask", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"fill_fraction_baseline:bid":         data.NewMetric[float64]("fill_fraction_baseline:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"fill_fraction_baseline:ask":         data.NewMetric[float64]("fill_fraction_baseline:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"fill_fraction_divergence:bid":       data.NewMetric[float64]("fill_fraction_divergence:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"fill_fraction_divergence:ask":       data.NewMetric[float64]("fill_fraction_divergence:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"fill_fraction_zscore:bid":           data.NewMetric[float64]("fill_fraction_zscore:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"fill_fraction_zscore:ask":           data.NewMetric[float64]("fill_fraction_zscore:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
	})
}
