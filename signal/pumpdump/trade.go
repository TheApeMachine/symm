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
	"github.com/theapemachine/symm/nomagique/temporal"
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
		hasTrade        bool
		prevTradeTime   time.Time
		barStartTime    time.Time
		targetQty       float64
		tradeCount      float64
		barQty          float64
		barNotional     float64
		barTradeCount   float64
		completedBars   float64
		barFromMidpoint float64
	}

	state := &tradeEntityState{}

	pipeline := nomagique.NewNumber(
		// 0. Extract source data and manage volume bar state
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

				var bid, ask float64
				if touchPeer := m.FindPeer(func(candidate *data.Measurement[float64]) bool {
					_, hasB := candidate.LookupMetric("best_bid")
					_, hasA := candidate.LookupMetric("best_ask")
					if !hasB {
						_, hasB = candidate.LookupMetric("bid")
					}
					if !hasA {
						_, hasA = candidate.LookupMetric("ask")
					}
					return hasB && hasA
				}); touchPeer != nil {
					bid = touchPeer.GetMetric("best_bid").Raw
					if bid == 0 {
						bid = touchPeer.GetMetric("bid").Raw
					}
					ask = touchPeer.GetMetric("best_ask").Raw
					if ask == 0 {
						ask = touchPeer.GetMetric("ask").Raw
					}
				}

				mid := 0.0
				if bid > 0 && ask > bid {
					mid = (bid + ask) / 2.0
				}

				if !state.hasTrade {
					state.targetQty = qty
					state.barStartTime = input.At
					if mid > 0 {
						state.barFromMidpoint = mid
					}
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
				m.WriteMetric("trade_quantity", qty)
				m.WriteMetric("trade_notional", notional)

				if hasInterval {
					m.WriteMetric("trade_interval_seconds", interval)
				}

				if hasInterval && duration > 0 && state.barQty >= state.targetQty {
					m.WriteMetric("volume_bar_target_quantity", state.targetQty)
					m.WriteMetric("volume_bar_quantity", state.barQty)
					m.WriteMetric("volume_bar_notional", state.barNotional)
					m.WriteMetric("volume_bar_trade_count", state.barTradeCount)
					m.WriteMetric("volume_bar_duration", duration)

					m.WriteMetric("volume_rate", state.barQty/duration)
					m.WriteMetric("notional_rate", state.barNotional/duration)
					m.WriteMetric("trade_rate", state.barTradeCount/duration)

					if state.barFromMidpoint > 0 && mid > 0 {
						m.WriteMetric("response_midpoint:from", state.barFromMidpoint)
						m.WriteMetric("response_midpoint:at", mid)
					}

					state.completedBars++
					m.WriteMetric("completed_bars", state.completedBars)

					// Reset the bar
					state.barQty = 0
					state.barNotional = 0
					state.barTradeCount = 0
					state.barStartTime = input.At
					state.barFromMidpoint = mid
				}

				m.Label = input.Label
				m.At = input.At
				return m
			},
			func(m *data.Measurement[float64], out *data.Measurement[float64]) {},
		),

		// 1. Adapter for log return (pure math, no state)
		data.NewAdapter(
			transport.NewPass(),
			func(m *data.Measurement[float64]) *data.Measurement[float64] {
				from, hasFrom := m.LookupMetric("response_midpoint:from")
				at, hasAt := m.LookupMetric("response_midpoint:at")
				if hasFrom && hasAt && from.Raw > 0 && at.Raw > 0 {
					logReturn := math.Log(at.Raw / from.Raw)
					m.WriteMetric("midpoint_log_return", logReturn)
					if duration, ok := m.LookupMetric("volume_bar_duration"); ok && duration.Raw > 0 {
						m.WriteMetric("midpoint_return_rate", logReturn/duration.Raw)
					}
				}
				return m
			},
			func(m *data.Measurement[float64], out *data.Measurement[float64]) {},
		),

		// 2. Compute advanced statistical baselines and velocities in parallel
		transport.NewFan(
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 { return m.GetMetric("notional_rate").Raw },
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("notional_rate_baseline", out.Baseline)
						if out.Baseline > 0 {
							ratio := m.GetMetric("notional_rate").Raw / out.Baseline
							m.WriteMetric("notional_rate_ratio", ratio)
							if ratio > 0 {
								div := math.Log(ratio)
								m.WriteMetric("notional_rate_divergence", div)
								m.SetMetadata(data.MetadataDivergence, strconv.FormatFloat(div, 'f', -1, 64))
							}
						}
						m.WriteMetric("notional_rate_zscore", out.ZScore)
						if out.VarianceDefined {
							m.SetMetadata(data.MetadataNoiseVariance, strconv.FormatFloat(out.Variance, 'f', -1, 64))
						}
					}
					m.EnsureMetadata()
					m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(out.Count, 'f', -1, 64))
				},
			),
			data.NewAdapter(
				temporal.NewVelocity(),
				func(m *data.Measurement[float64]) temporal.Observation {
					if rate, ok := m.LookupMetric("notional_rate"); ok {
						return temporal.Observation{Value: rate.Raw, At: m.At.UnixNano()}
					}
					return temporal.Observation{Value: math.NaN(), At: m.At.UnixNano()}
				},
				func(m *data.Measurement[float64], out temporal.VelocityReading) {
					if out.Defined {
						m.WriteMetric("notional_rate_velocity", out.Rate)
					}
				},
			),
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 { return m.GetMetric("midpoint_return_rate").Raw },
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("midpoint_return_baseline", out.Baseline)
						m.WriteMetric("midpoint_return_divergence", out.Residual)
						m.WriteMetric("midpoint_return_zscore", out.ZScore)
					}
				},
			),
			data.NewAdapter(
				temporal.NewVelocity(),
				func(m *data.Measurement[float64]) temporal.Observation {
					if rate, ok := m.LookupMetric("midpoint_return_rate"); ok {
						return temporal.Observation{Value: rate.Raw, At: m.At.UnixNano()}
					}
					return temporal.Observation{Value: math.NaN(), At: m.At.UnixNano()}
				},
				func(m *data.Measurement[float64], out temporal.VelocityReading) {
					if out.Defined {
						m.WriteMetric("midpoint_return_velocity", out.Rate)
					}
				},
			),
		),

		// 3. Finalize
		data.NewFinalizer[float64](),
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

	measurement.SetSource("pumpdump:trade")

	res := data.Read[*data.Measurement[float64]](trade.pipelineFor(measurement.Label).Next(
		transport.NewOne(unsafe.Pointer(&measurement)).Next(nil),
	))

	if res == nil {
		return nil
	}

	res.Finalize()
	return res
}
