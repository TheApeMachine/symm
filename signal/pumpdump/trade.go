package pumpdump

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Trade owns the volume-clock activity pipeline. It holds no state and no logic of
its own: its entire behavior is one nomagique pipeline over the measurement itself —
every stage writes its facts into the measurement where it computes them, and the
workload's register owns the measurement's lifetime.
*/
type Trade struct {
	*runtime.System
	pipelines sync.Map
	ID        int
}

func NewTrade(ctx context.Context) *Trade {
	trade := &Trade{}

	trade.System = runtime.NewSystem(ctx, "pumpdump:trade", trade)
	return trade
}

func (trade *Trade) pipelineFor(symbol string) core.Primitive {
	if existing, ok := trade.pipelines.Load(symbol); ok {
		return existing.(core.Primitive)
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

	state := &tradeEntityState{
		notionalBaseline: adaptive.NewBaseline(adaptive.NewWindow()),
	}

	pipeline := nomagique.NewNumber(
		data.NewAdapter(
			transport.NewPass(),
			func(m *data.Measurement[float64]) *data.Measurement[float64] {
				input := m
				if len(m.Peers) > 0 {
					peer := m.FindPeer(func(p *data.Measurement[float64]) bool {
						_, hasP := p.LookupMetric("price")
						_, hasQ := p.LookupMetric("qty")
						return hasP && hasQ && p.Label != ""
					})
					if peer != nil {
						input = peer
					}
				}
				m.Pull(input)

				priceMetric, hasPrice := input.LookupMetric("price")
				qtyMetric, hasQty := input.LookupMetric("qty")

				if !hasPrice || !hasQty || priceMetric.Raw <= 0 || qtyMetric.Raw <= 0 {
					m.Err = fmt.Errorf("pumpdump: non-positive price or quantity")
					return m
				}

				price := priceMetric.Raw
				qty := qtyMetric.Raw
				notional := price * qty

				if !state.hasTrade {
					state.targetQty = qty
					state.barStartTime = input.At
				} else {
					state.targetQty = (state.targetQty*state.tradeCount + qty) / (state.tradeCount + 1)
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

				state.barQty += qty
				state.barNotional += notional
				state.barTradeCount++

				duration := input.At.Sub(state.barStartTime).Seconds()

				m.WriteMetric("trade_price", price)
				m.WriteMetric("trade_qty", qty)
				m.WriteMetric("trade_notional", notional)
				m.WriteMetric("target_qty", state.targetQty)
				m.WriteMetric("bar_qty", state.barQty)
				m.WriteMetric("bar_notional", state.barNotional)
				m.WriteMetric("bar_trade_count", state.barTradeCount)
				m.WriteMetric("bar_duration", duration)

				if hasInterval {
					m.WriteMetric("trade_interval", interval)
				}

				if hasInterval && duration > 0 && state.barQty >= state.targetQty {
					volumeRate := state.barQty / duration
					notionalRate := state.barNotional / duration
					tradeRate := state.barTradeCount / duration
					state.completedBars++

					m.WriteMetric("volume_rate", volumeRate)
					m.WriteMetric("notional_rate", notionalRate)
					m.WriteMetric("trade_rate", tradeRate)
					m.WriteMetric("completed_bars", state.completedBars)

					var reading adaptive.BaselineReading
					for rPtr := range state.notionalBaseline.Next(transport.NewOne(unsafe.Pointer(&notionalRate)).Next(nil)) {
						reading = *(*adaptive.BaselineReading)(rPtr)
					}

					m.WriteMetric("notional_rate_baseline", reading.Baseline)

					m.EnsureMetadata()
					m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(reading.Count, 'f', -1, 64))

					if reading.Baseline > 0 {
						notionalRatio := notionalRate / reading.Baseline
						m.WriteMetric("notional_rate_ratio", notionalRatio)
						if reading.HasPrior {
							divergence := math.Log(notionalRatio)
							m.WriteMetric("notional_rate_divergence", divergence)
							m.SetMetadata(data.MetadataDivergence, strconv.FormatFloat(divergence, 'f', -1, 64))
						}
					}

					m.WriteMetric("notional_rate_zscore", reading.ZScore)
					if reading.VarianceDefined {
						m.SetMetadata(data.MetadataNoiseVariance, strconv.FormatFloat(reading.Variance, 'f', -1, 64))
					}

					// Reset the bar
					state.barQty = 0
					state.barNotional = 0
					state.barTradeCount = 0
					state.barStartTime = input.At
				}

				m.Label = input.Label
				m.At = input.At
				return m
			},
			func(m *data.Measurement[float64], out *data.Measurement[float64]) {},
		),
	)

	actual, _ := trade.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/
func (trade *Trade) Step(measurement *data.Measurement[float64]) *data.Measurement[float64] {
	if trade.Status() != runtime.READY {
		errnie.Warn(trade.Name() + ": Step called before READY; dropping event")
		return measurement
	}

	if measurement == nil || measurement.Err != nil {
		return measurement
	}

	measurement.Source = "pumpdump:trade"

	res := data.Read[*data.Measurement[float64]](trade.pipelineFor(measurement.Label).Next(
		transport.NewOne(unsafe.Pointer(&measurement)).Next(nil),
	))

	if res == nil {
		return nil
	}

	res.Finalize()
	return res
}
