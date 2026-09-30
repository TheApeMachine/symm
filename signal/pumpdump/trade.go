package pumpdump

import (
	"context"
	"fmt"
	"github.com/theapemachine/errnie"
	"iter"
	"strconv"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

type tradeEntityInput struct {
	Symbol string
	Price  float64
	Qty    float64
	At     time.Time
}

type tradeEntityResult struct {
	TradePrice        float64
	TradeQty          float64
	TradeNotional     float64
	TargetQty         float64
	BarQty            float64
	BarNotional       float64
	BarTradeCount     float64
	BarDuration       float64
	Interval          float64
	HasInterval       bool
	VolumeRate        float64
	NotionalRate      float64
	TradeRate         float64
	HasRates          bool
	CompletedBars     float64
	NotionalReading   adaptive.BaselineReading
	NotionalRateRatio float64
}

type tradeEntityState struct {
	hasTrade         bool
	prevTradeTime    time.Time
	barStartTime     time.Time
	targetQty        float64
	tradeCount       float64
	barQty           float64
	barNotional      float64
	barTradeCount    float64
	completedBars    float64
	notionalBaseline core.Primitive
}

type tradeEntityPipeline struct {
	*core.PrimitiveError
	paths map[string]*tradeEntityState
	out   tradeEntityResult
}

func newTradeEntityPipeline() core.Primitive {
	return &tradeEntityPipeline{
		PrimitiveError: core.NewPrimitiveError(),
		paths:          make(map[string]*tradeEntityState),
	}
}

func (op *tradeEntityPipeline) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*tradeEntityInput)(arriving)

			state := op.paths[input.Symbol]
			if state == nil {
				state = &tradeEntityState{
					notionalBaseline: adaptive.NewBaseline(adaptive.NewWindow()),
				}
				op.paths[input.Symbol] = state
			}

			notional := input.Price * input.Qty

			if !state.hasTrade {
				state.targetQty = input.Qty
				state.barStartTime = input.At
			}
			if state.hasTrade {
				state.targetQty = (state.targetQty*state.tradeCount + input.Qty) / (state.tradeCount + 1)
			}
			state.tradeCount++

			var interval float64
			var hasInterval bool

			if state.hasTrade {
				interval = input.At.Sub(state.prevTradeTime).Seconds()
				hasInterval = true
			}

			state.prevTradeTime = input.At
			state.hasTrade = true

			state.barQty += input.Qty
			state.barNotional += notional
			state.barTradeCount++

			duration := input.At.Sub(state.barStartTime).Seconds()

			var volumeRate, notionalRate, tradeRate float64
			var hasRates bool
			var reading adaptive.BaselineReading
			var notionalRatio float64

			if hasInterval && duration > 0 && state.barQty >= state.targetQty {
				volumeRate = state.barQty / duration
				notionalRate = state.barNotional / duration
				tradeRate = state.barTradeCount / duration
				hasRates = true
				state.completedBars++

				for rPtr := range state.notionalBaseline.Next(transport.NewOne(unsafe.Pointer(&notionalRate)).Next(nil)) {
					reading = *(*adaptive.BaselineReading)(rPtr)
				}

				notionalRatio = 1.0
				if reading.Baseline > 0 {
					notionalRatio = notionalRate / reading.Baseline
				}
			}

			op.out = tradeEntityResult{
				TradePrice:        input.Price,
				TradeQty:          input.Qty,
				TradeNotional:     notional,
				TargetQty:         state.targetQty,
				BarQty:            state.barQty,
				BarNotional:       state.barNotional,
				BarTradeCount:     state.barTradeCount,
				BarDuration:       duration,
				Interval:          interval,
				HasInterval:       hasInterval,
				VolumeRate:        volumeRate,
				NotionalRate:      notionalRate,
				TradeRate:         tradeRate,
				HasRates:          hasRates,
				CompletedBars:     state.completedBars,
				NotionalReading:   reading,
				NotionalRateRatio: notionalRatio,
			}

			if hasRates {
				state.barQty = 0
				state.barNotional = 0
				state.barTradeCount = 0
				state.barStartTime = input.At
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Trade owns the volume-clock activity pipeline. It holds no state and no logic of
its own: its entire behavior is one nomagique pipeline over the measurement itself —
every stage writes its facts into the measurement where it computes them, and the
workload's register owns the measurement's lifetime.
*/
type Trade struct {
	*runtime.System
	pipeline core.Primitive
	ID       int
}

func NewTrade(ctx context.Context) *Trade {
	trade := &Trade{
		pipeline: nomagique.NewNumber(newTradeEntityPipeline()),
	}

	trade.System = runtime.NewSystem(ctx, "pumpdump:trade", trade)
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

	m.Pull(input)

	priceMetric, hasPrice := input.Metrics["price"]
	qtyMetric, hasQty := input.Metrics["qty"]

	if !hasPrice || !hasQty {
		return m
	}

	price := priceMetric.Raw
	qty := qtyMetric.Raw

	if price <= 0 || qty <= 0 {
		m.Err = fmt.Errorf("pumpdump: non-positive price or quantity")
		return m
	}

	if m.Metadata == nil {
		m.Metadata = make(map[string]string)
	}

	pipeInput := tradeEntityInput{
		Symbol: input.Label,
		Price:  price,
		Qty:    qty,
		At:     input.At,
	}

	for out := range trade.pipeline.Next(transport.NewOne(unsafe.Pointer(&pipeInput)).Next(nil)) {
		res := (*tradeEntityResult)(out)

		m.Metrics["trade_price"] = m.Metrics["trade_price"].Write(res.TradePrice)
		m.Metrics["trade_quantity"] = m.Metrics["trade_quantity"].Write(res.TradeQty)
		m.Metrics["trade_notional"] = m.Metrics["trade_notional"].Write(res.TradeNotional)
		m.Metrics["volume_bar_target_quantity"] = m.Metrics["volume_bar_target_quantity"].Write(res.TargetQty)
		m.Metrics["volume_bar_quantity"] = m.Metrics["volume_bar_quantity"].Write(res.BarQty)
		m.Metrics["volume_bar_notional"] = m.Metrics["volume_bar_notional"].Write(res.BarNotional)
		m.Metrics["volume_bar_trade_count"] = m.Metrics["volume_bar_trade_count"].Write(res.BarTradeCount)
		m.Metrics["volume_bar_duration"] = m.Metrics["volume_bar_duration"].Write(res.BarDuration)

		if res.HasInterval {
			m.Metrics["trade_interval_seconds"] = m.Metrics["trade_interval_seconds"].Write(res.Interval)
		}

		if res.HasRates {
			m.Metrics["volume_rate"] = m.Metrics["volume_rate"].Write(res.VolumeRate)
			m.Metrics["notional_rate"] = m.Metrics["notional_rate"].Write(res.NotionalRate)
			m.Metrics["trade_rate"] = m.Metrics["trade_rate"].Write(res.TradeRate)
			m.Metrics["completed_volume_bar_ordinal"] = m.Metrics["completed_volume_bar_ordinal"].Write(res.CompletedBars)
			m.Metrics["notional_rate_baseline"] = m.Metrics["notional_rate_baseline"].Write(res.NotionalReading.Baseline)
			m.Metrics["notional_rate_ratio"] = m.Metrics["notional_rate_ratio"].Write(res.NotionalRateRatio)

			m.Metadata[data.MetadataSupport] = strconv.FormatFloat(res.NotionalReading.Count, 'f', -1, 64)

			if res.NotionalReading.HasPrior {
				m.Metrics["notional_rate_divergence"] = m.Metrics["notional_rate_divergence"].Write(res.NotionalReading.Residual)
				m.Metrics["notional_rate_zscore"] = m.Metrics["notional_rate_zscore"].Write(res.NotionalReading.ZScore)
				m.Metadata[data.MetadataDivergence] = strconv.FormatFloat(res.NotionalReading.Residual, 'f', -1, 64)

				if res.NotionalReading.VarianceDefined {
					m.Metadata[data.MetadataNoiseVariance] = strconv.FormatFloat(res.NotionalReading.Variance, 'f', -1, 64)
				}
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
	m := data.NewMeasurement[float64]("pumpdump:trade", map[string]data.Metric[float64]{
		"trade_price":                  data.NewMetric[float64]("trade_price", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"trade_quantity":               data.NewMetric[float64]("trade_quantity", data.UnitCount, data.TimescaleInstantaneous, 0, 0),
		"trade_notional":               data.NewMetric[float64]("trade_notional", data.UnitCount, data.TimescaleInstantaneous, 0, 0),
		"volume_bar_target_quantity":   data.NewMetric[float64]("volume_bar_target_quantity", data.UnitCount, data.TimescaleInstantaneous, 0, 0),
		"volume_bar_quantity":          data.NewMetric[float64]("volume_bar_quantity", data.UnitCount, data.TimescaleInstantaneous, 0, 0),
		"volume_bar_notional":          data.NewMetric[float64]("volume_bar_notional", data.UnitCount, data.TimescaleInstantaneous, 0, 0),
		"volume_bar_trade_count":       data.NewMetric[float64]("volume_bar_trade_count", data.UnitCount, data.TimescaleInstantaneous, 0, 0),
		"volume_bar_duration":          data.NewMetric[float64]("volume_bar_duration", data.UnitSecond, data.TimescaleInstantaneous, 0, 0),
		"completed_volume_bar_ordinal": data.NewMetric[float64]("completed_volume_bar_ordinal", data.UnitCount, data.TimescaleInstantaneous, 0, 0),
		"trade_interval_seconds":       data.NewMetric[float64]("trade_interval_seconds", data.UnitSecond, data.TimescaleInstantaneous, 0, 0),
		"volume_rate":                  data.NewMetric[float64]("volume_rate", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"notional_rate":                data.NewMetric[float64]("notional_rate", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"trade_rate":                   data.NewMetric[float64]("trade_rate", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"notional_rate_baseline":       data.NewMetric[float64]("notional_rate_baseline", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"notional_rate_ratio":          data.NewMetric[float64]("notional_rate_ratio", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 0),
		"notional_rate_divergence":     data.NewMetric[float64]("notional_rate_divergence", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"notional_rate_zscore":         data.NewMetric[float64]("notional_rate_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
	})
	m.Metadata["peer-interest"] = "*"
	return m
}
