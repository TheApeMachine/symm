package toxicity

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
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
				bidPrice := m.GetMetric("best_price:bid").Raw
				if bidPrice == 0 {
					bidPrice = m.GetMetric("best_bid").Raw
				}
				if bidPrice == 0 {
					bidPrice = m.GetMetric("bid").Raw
				}

				askPrice := m.GetMetric("best_price:ask").Raw
				if askPrice == 0 {
					askPrice = m.GetMetric("best_ask").Raw
				}
				if askPrice == 0 {
					askPrice = m.GetMetric("ask").Raw
				}

				bidQty := m.GetMetric("touch_quantity:bid").Raw
				if bidQty == 0 {
					bidQty = m.GetMetric("bid_qty").Raw
				}

				askQty := m.GetMetric("touch_quantity:ask").Raw
				if askQty == 0 {
					askQty = m.GetMetric("ask_qty").Raw
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
					// Only stamp From when the prior touch is not after At.
					if !state.prevTime.IsZero() && !state.prevTime.After(m.At) {
						m.From = state.prevTime
					}
					m.WriteMetric("previous_touch_quantity:bid", state.prevBidQty)
					m.WriteMetric("previous_touch_quantity:ask", state.prevAskQty)
					m.EnsureMetadata()
					m.SetMetadata("previous_level_disposition", "touch-only")

					m.WriteMetric("previous_best_price:bid", state.prevBid)
					m.WriteMetric("previous_best_price:ask", state.prevAsk)

					dt := 0.0
					if !m.From.IsZero() {
						dt = m.At.Sub(m.From).Seconds()
					}

					if state.prevBid > 0 && bidPrice > 0 {
						m.WriteMetric("touch_price_log_change:bid", math.Log(bidPrice/state.prevBid))
					}

					if state.prevAsk > 0 && askPrice > 0 {
						m.WriteMetric("touch_price_log_change:ask", math.Log(askPrice/state.prevAsk))
					}

					if bidPrice < state.prevBid {
						m.WriteMetric("retreated_quantity:bid", state.prevBidQty)
						m.WriteNormalized("retreat_fraction:bid", 1.0)
						if dt > 0 {
							m.WriteMetric("retreat_rate:bid", state.prevBidQty/dt)
						}
					}

					if bidPrice == state.prevBid {
						if bidQty < state.prevBidQty {
							withdrawn := state.prevBidQty - bidQty
							m.WriteMetric("net_withdrawn_quantity:bid", withdrawn)
							if state.prevBidQty > 0 {
								m.WriteNormalized("net_withdrawal_fraction:bid", withdrawn/state.prevBidQty)
							}
							if dt > 0 {
								m.WriteMetric("net_withdrawal_rate:bid", withdrawn/dt)
							}
						}
						if bidQty > state.prevBidQty {
							replenished := bidQty - state.prevBidQty
							m.WriteMetric("net_replenished_quantity:bid", replenished)
							if state.prevBidQty > 0 {
								m.WriteNormalized("net_replenishment_fraction:bid", replenished/state.prevBidQty)
							}
							if dt > 0 {
								m.WriteMetric("net_replenishment_rate:bid", replenished/dt)
							}
						}
					}

					if askPrice > state.prevAsk {
						m.WriteMetric("retreated_quantity:ask", state.prevAskQty)
						m.WriteNormalized("retreat_fraction:ask", 1.0)
						if dt > 0 {
							m.WriteMetric("retreat_rate:ask", state.prevAskQty/dt)
						}
					}

					if askPrice == state.prevAsk {
						if askQty < state.prevAskQty {
							withdrawn := state.prevAskQty - askQty
							m.WriteMetric("net_withdrawn_quantity:ask", withdrawn)
							if state.prevAskQty > 0 {
								m.WriteNormalized("net_withdrawal_fraction:ask", withdrawn/state.prevAskQty)
							}
							if dt > 0 {
								m.WriteMetric("net_withdrawal_rate:ask", withdrawn/dt)
							}
						}
						if askQty > state.prevAskQty {
							replenished := askQty - state.prevAskQty
							m.WriteMetric("net_replenished_quantity:ask", replenished)
							if state.prevAskQty > 0 {
								m.WriteNormalized("net_replenishment_fraction:ask", replenished/state.prevAskQty)
							}
							if dt > 0 {
								m.WriteMetric("net_replenishment_rate:ask", replenished/dt)
							}
						}
					}
				}

				state.prevTime = m.At
				state.prevBid = bidPrice
				state.prevAsk = askPrice
				state.prevBidQty = bidQty
				state.prevAskQty = askQty
				state.hasPrev = true

				return m
			},
			func(m *data.Measurement[float64], res *data.Measurement[float64]) {},
		),
		// 1. Baselines
		transport.NewFan(
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("net_withdrawal_fraction:bid").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("withdrawal_fraction_baseline:bid", out.Baseline)
						m.WriteMetric("withdrawal_fraction_divergence:bid", out.Residual)
						m.WriteStandardized("withdrawal_fraction_zscore:bid", out.ZScore)
					}
				},
			),
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("net_withdrawal_fraction:ask").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("withdrawal_fraction_baseline:ask", out.Baseline)
						m.WriteMetric("withdrawal_fraction_divergence:ask", out.Residual)
						m.WriteStandardized("withdrawal_fraction_zscore:ask", out.ZScore)
					}
				},
			),
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("retreat_fraction:bid").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("retreat_fraction_baseline:bid", out.Baseline)
						m.WriteStandardized("retreat_fraction_zscore:bid", out.ZScore)
					}
				},
			),
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("retreat_fraction:ask").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("retreat_fraction_baseline:ask", out.Baseline)
						m.WriteStandardized("retreat_fraction_zscore:ask", out.ZScore)
					}
				},
			),
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("net_replenishment_fraction:bid").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("replenishment_fraction_baseline:bid", out.Baseline)
					}
				},
			),
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("net_replenishment_fraction:ask").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("replenishment_fraction_baseline:ask", out.Baseline)
					}
				},
			),
			data.NewAdapter(
				statistic.NewJoint(4),
				func(m *data.Measurement[float64]) statistic.JointInput {
					wb := m.GetMetric("withdrawal_fraction_divergence:bid").Raw
					wa := m.GetMetric("withdrawal_fraction_divergence:ask").Raw
					rb := m.GetMetric("retreat_fraction_zscore:bid").Raw
					ra := m.GetMetric("retreat_fraction_zscore:ask").Raw
					if wb == 0 && wa == 0 && rb == 0 && ra == 0 {
						return statistic.JointInput{Values: nil}
					}
					return statistic.JointInput{Values: []float64{wb, wa, rb, ra}}
				},
				func(m *data.Measurement[float64], out statistic.JointReading) {
					if out.SNRDefined && out.SNR < 1/math.Sqrt(2.220446049250313e-16) {
						m.WriteMetric("SNR", out.SNR)
						m.EnsureMetadata()
						m.SetMetadata(data.MetadataMahalanobisSNR, strconv.FormatFloat(out.SNR, 'f', -1, 64))
					}
					if len(out.Channels) > 0 {
						n := out.Channels[0].Count
						maturity := 0.0
						if n > 1 {
							maturity = 1.0 - (1.0 / n)
						}
						m.WriteNormalized("Maturity", maturity)
					}
				},
			),
		),
		// 2. Finalize
		data.NewFinalizer[float64](),
	)

	actual, _ := level3.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

func (level3 *Level3) Step(input *runtime.StageInput, output *data.Measurement[float64]) *data.Measurement[float64] {
	if level3.Status() != runtime.READY {
		errnie.Warn(level3.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if input == nil || input.Ingress() == nil || input.Symbol() == "" {
		return nil
	}

	ingress := input.Ingress()
	scratch := data.NewMeasurement[float64]("toxicity:scratch", nil)
	scratch.Label = output.Label

	if ingress.Metrics != nil {
		for key, metric := range ingress.Metrics {
			scratch.SetMetric(key, metric)
		}
	}

	if channel, hasCh := input.IngressProvenance("channel"); hasCh {
		scratch.SetProvenance("channel", channel)
	}

	data.StampInterval(scratch, input.At(), input.From())

	res := data.Read[*data.Measurement[float64]](level3.pipelineFor(output.Label).Next(
		transport.NewOne(unsafe.Pointer(&scratch)).Next(nil),
	))

	if res != nil {
		exclude := make([]string, 0, len(ingress.Metrics))
		for k := range ingress.Metrics {
			exclude = append(exclude, k)
		}
		data.CopyProducedFacts(res, output, exclude...)
	}

	return output
}
