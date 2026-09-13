package pumpdump

import (
	"context"
	"fmt"
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

type tradeEntityInput struct {
	Price float64
	Qty   float64
	At    time.Time
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

type tradeEntityPipeline struct {
	*core.PrimitiveError
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
	out              tradeEntityResult
}

func newTradeEntityPipeline() core.Primitive {
	return &tradeEntityPipeline{
		PrimitiveError:   core.NewPrimitiveError(),
		notionalBaseline: adaptive.NewBaseline(adaptive.NewWindow()),
	}
}

func (op *tradeEntityPipeline) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*tradeEntityInput)(arriving)

			notional := input.Price * input.Qty

			if !op.hasTrade {
				op.targetQty = input.Qty
				op.barStartTime = input.At
			}
			if op.hasTrade {
				op.targetQty = (op.targetQty*op.tradeCount + input.Qty) / (op.tradeCount + 1)
			}
			op.tradeCount++

			var interval float64
			var hasInterval bool

			if op.hasTrade {
				interval = input.At.Sub(op.prevTradeTime).Seconds()
				hasInterval = true
			}

			op.prevTradeTime = input.At
			op.hasTrade = true

			op.barQty += input.Qty
			op.barNotional += notional
			op.barTradeCount++

			duration := input.At.Sub(op.barStartTime).Seconds()

			var volumeRate, notionalRate, tradeRate float64
			var hasRates bool
			var reading adaptive.BaselineReading
			var notionalRatio float64

			if hasInterval && duration > 0 && op.barQty >= op.targetQty {
				volumeRate = op.barQty / duration
				notionalRate = op.barNotional / duration
				tradeRate = op.barTradeCount / duration
				hasRates = true
				op.completedBars++

				for rPtr := range op.notionalBaseline.Next(transport.NewOne(unsafe.Pointer(&notionalRate)).Next(nil)) {
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
				TargetQty:         op.targetQty,
				BarQty:            op.barQty,
				BarNotional:       op.barNotional,
				BarTradeCount:     op.barTradeCount,
				BarDuration:       duration,
				Interval:          interval,
				HasInterval:       hasInterval,
				VolumeRate:        volumeRate,
				NotionalRate:      notionalRate,
				TradeRate:         tradeRate,
				HasRates:          hasRates,
				CompletedBars:     op.completedBars,
				NotionalReading:   reading,
				NotionalRateRatio: notionalRatio,
			}

			if hasRates {
				op.barQty = 0
				op.barNotional = 0
				op.barTradeCount = 0
				op.barStartTime = input.At
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
	return &Trade{
		System:   runtime.NewSystem(ctx, "pumpdump:trade"),
		pipeline: nomagique.NewNumber(newTradeEntityPipeline()),
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

	price := m.Metrics["price"].Raw
	qty := m.Metrics["qty"].Raw

	if price <= 0 || qty <= 0 {
		m.Err = fmt.Errorf("pumpdump: non-positive price or quantity")
		return m
	}

	if m.Metadata == nil {
		m.Metadata = make(map[string]float64)
	}

	input := tradeEntityInput{
		Price: price,
		Qty:   qty,
		At:    m.At,
	}

	for out := range trade.pipeline.Next(transport.NewOne(unsafe.Pointer(&input)).Next(nil)) {
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

			m.Metadata[data.MetadataSupport] = res.NotionalReading.Count

			if res.NotionalReading.HasPrior {
				m.Metrics["notional_rate_divergence"] = m.Metrics["notional_rate_divergence"].Write(res.NotionalReading.Residual)
				m.Metrics["notional_rate_zscore"] = m.Metrics["notional_rate_zscore"].Write(res.NotionalReading.ZScore)
				m.Metadata[data.MetadataDivergence] = res.NotionalReading.Residual

				if res.NotionalReading.VarianceDefined {
					m.Metadata[data.MetadataNoiseVariance] = res.NotionalReading.Variance
				}
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
	return data.NewMeasurement[float64]("pumpdump:trade", map[string]data.Metric[float64]{
		"trade_price":                  data.NewMetric[float64]("trade_price", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"trade_quantity":               data.NewMetric[float64]("trade_quantity", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"trade_notional":               data.NewMetric[float64]("trade_notional", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"volume_bar_target_quantity":   data.NewMetric[float64]("volume_bar_target_quantity", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"volume_bar_quantity":          data.NewMetric[float64]("volume_bar_quantity", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"volume_bar_notional":          data.NewMetric[float64]("volume_bar_notional", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"volume_bar_trade_count":       data.NewMetric[float64]("volume_bar_trade_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"volume_bar_duration":          data.NewMetric[float64]("volume_bar_duration", data.UnitSecond, data.TimescaleInstantaneous, 0, 1),
		"completed_volume_bar_ordinal": data.NewMetric[float64]("completed_volume_bar_ordinal", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"trade_interval_seconds":       data.NewMetric[float64]("trade_interval_seconds", data.UnitSecond, data.TimescaleInstantaneous, 0, 1),
		"volume_rate":                  data.NewMetric[float64]("volume_rate", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"notional_rate":                data.NewMetric[float64]("notional_rate", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"trade_rate":                   data.NewMetric[float64]("trade_rate", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"notional_rate_baseline":       data.NewMetric[float64]("notional_rate_baseline", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"notional_rate_ratio":          data.NewMetric[float64]("notional_rate_ratio", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"notional_rate_divergence":     data.NewMetric[float64]("notional_rate_divergence", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"notional_rate_zscore":         data.NewMetric[float64]("notional_rate_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
	})
}
