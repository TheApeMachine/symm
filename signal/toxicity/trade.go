package toxicity

import (
	"context"
	"iter"
	"strconv"
	"time"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/runtime"
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
				for rPtr := range op.bidBaseline.Next(sequence.NewOne(unsafe.Pointer(&bidFillFrac)).Next(nil)) {
					bidReading = *(*adaptive.BaselineReading)(rPtr)
				}
				op.bidFractionSamples++
			}

			if askFillFrac > 0 {
				for rPtr := range op.askBaseline.Next(sequence.NewOne(unsafe.Pointer(&askFillFrac)).Next(nil)) {
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
	trade := &Trade{
		pipeline: nomagique.NewNumber(newTradePipeline()),
	}

	trade.System = runtime.NewSystem(ctx, "toxicity:trade", trade)
	return trade
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/
func (trade *Trade) Step(m *data.Measurement[float64]) *data.Measurement[float64] {
	if trade.Status() != runtime.READY {
		errnie.Warn(trade.Name() + ": Step called before READY; dropping event")
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
			_, hasP := p.Metrics["price"]
			_, hasQ := p.Metrics["qty"]
			return hasP && hasQ && p.Label != ""
		})

		if peer == nil {
			return nil
		}

		input = peer
	}

	price := input.Metrics["price"].Raw
	qty := input.Metrics["qty"].Raw

	if price <= 0 || qty <= 0 {
		return m
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

	if bidPrice == 0 || askPrice == 0 {
		touchPeer := m.FindPeer(func(p *data.Measurement[float64]) bool {
			if p.Label == "" {
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

		if touchPeer != nil {
			if bidPrice == 0 {
				bidPrice = touchPeer.Metrics["best_price:bid"].Raw
				if bidPrice == 0 {
					bidPrice = touchPeer.Metrics["best_bid"].Raw
				}
				if bidPrice == 0 {
					bidPrice = touchPeer.Metrics["bid"].Raw
				}
			}
			if askPrice == 0 {
				askPrice = touchPeer.Metrics["best_price:ask"].Raw
				if askPrice == 0 {
					askPrice = touchPeer.Metrics["best_ask"].Raw
				}
				if askPrice == 0 {
					askPrice = touchPeer.Metrics["ask"].Raw
				}
			}
			if bidQty == 0 {
				bidQty = touchPeer.Metrics["touch_quantity:bid"].Raw
				if bidQty == 0 {
					bidQty = touchPeer.Metrics["bid_qty"].Raw
				}
			}
			if askQty == 0 {
				askQty = touchPeer.Metrics["touch_quantity:ask"].Raw
				if askQty == 0 {
					askQty = touchPeer.Metrics["ask_qty"].Raw
				}
			}
		}
	}

	pipeInput := tradeInput{
		Price:    price,
		Qty:      qty,
		Side:     input.Provenance["side"],
		BidPrice: bidPrice,
		AskPrice: askPrice,
		BidQty:   bidQty,
		AskQty:   askQty,
		At:       input.At,
	}

	for out := range trade.pipeline.Next(sequence.NewOne(unsafe.Pointer(&pipeInput)).Next(nil)) {
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
			m.Metadata[data.MetadataSupport] = strconv.FormatFloat(res.BidReading.Count, 'f', -1, 64)
			m.Metadata[data.MetadataDivergence] = strconv.FormatFloat(res.BidReading.Residual, 'f', -1, 64)

			if res.BidReading.VarianceDefined {
				m.Metadata[data.MetadataNoiseVariance] = strconv.FormatFloat(res.BidReading.Variance, 'f', -1, 64)
			}
		}

		if res.AskSupported {
			m.Metadata[data.MetadataSupport] = strconv.FormatFloat(res.AskReading.Count, 'f', -1, 64)
			m.Metadata[data.MetadataDivergence] = strconv.FormatFloat(res.AskReading.Residual, 'f', -1, 64)

			if res.AskReading.VarianceDefined {
				m.Metadata[data.MetadataNoiseVariance] = strconv.FormatFloat(res.AskReading.Variance, 'f', -1, 64)
			}
		}
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
func (trade *Trade) Register() *data.Measurement[float64] {
	m := data.NewMeasurement("toxicity:trade", map[string]data.Metric[float64]{
		"bracket_trade_quantity":           data.NewMetric[float64]("bracket_trade_quantity", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"matched_touch_trade_quantity:bid": data.NewMetric[float64]("matched_touch_trade_quantity:bid", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"matched_touch_trade_quantity:ask": data.NewMetric[float64]("matched_touch_trade_quantity:ask", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"touch_fill_quantity:bid":          data.NewMetric[float64]("touch_fill_quantity:bid", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"touch_fill_quantity:ask":          data.NewMetric[float64]("touch_fill_quantity:ask", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"touch_fill_fraction:bid":          data.NewMetric[float64]("touch_fill_fraction:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"touch_fill_fraction:ask":          data.NewMetric[float64]("touch_fill_fraction:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"touch_fill_rate:bid":              data.NewMetric[float64]("touch_fill_rate:bid", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"touch_fill_rate:ask":              data.NewMetric[float64]("touch_fill_rate:ask", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"fill_fraction_baseline:bid":       data.NewMetric[float64]("fill_fraction_baseline:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"fill_fraction_baseline:ask":       data.NewMetric[float64]("fill_fraction_baseline:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"fill_fraction_divergence:bid":     data.NewMetric[float64]("fill_fraction_divergence:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"fill_fraction_divergence:ask":     data.NewMetric[float64]("fill_fraction_divergence:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"fill_fraction_zscore:bid":         data.NewMetric[float64]("fill_fraction_zscore:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"fill_fraction_zscore:ask":         data.NewMetric[float64]("fill_fraction_zscore:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
	})
	m.Metadata["peer-interest"] = "*"
	return m
}
