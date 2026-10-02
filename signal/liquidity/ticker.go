package liquidity

import (
	"context"
	"strconv"
	"sync"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	nmliquidity "github.com/theapemachine/symm/nomagique/liquidity"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

type Ticker struct {
	*runtime.System
	arena     *data.ArenaOwner
	pipelines sync.Map
	ID        int
}

func NewTicker(ctx context.Context, arena *data.ArenaOwner) *Ticker {
	ticker := &Ticker{
		arena: arena,
	}

	ticker.System = runtime.NewSystem(ctx, "liquidity:ticker", ticker)
	return ticker
}

func (ticker *Ticker) Source() string {
	return "liquidity:ticker"
}

func (ticker *Ticker) Arena() *data.ArenaOwner {
	return ticker.arena
}

func (ticker *Ticker) pipelineFor(symbol string) core.Primitive {
	if existing, ok := ticker.pipelines.Load(symbol); ok {
		return existing.(core.Primitive)
	}

	pipeline := nomagique.NewNumber(
		nmliquidity.NewGate(),
		nmliquidity.NewTouch(),
		transport.NewFan(
			data.NewAdapter(
				statistic.NewJoint(3),
				func(m *data.Measurement[float64]) statistic.JointInput {
					return statistic.JointInput{Values: []float64{
						m.GetMetric("_log_bid_notional").Raw,
						m.GetMetric("_log_ask_notional").Raw,
						m.GetMetric("_log_relative_spread").Raw,
					}}
				},
				func(m *data.Measurement[float64], reading statistic.JointReading) {
					if len(reading.Channels) > 0 {
						m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(reading.Channels[0].Count, 'f', -1, 64))
					}
					if reading.SNRDefined {
						m.SetMetadata(data.MetadataMahalanobisSNR, strconv.FormatFloat(reading.SNR, 'f', -1, 64))
					}

					originals := []float64{
						m.GetMetric("touch_notional:bid").Raw,
						m.GetMetric("touch_notional:ask").Raw,
						m.GetMetric("relative_spread").Raw,
					}

					baselineLabels := []string{"touch_notional_baseline:bid", "touch_notional_baseline:ask", "relative_spread_baseline"}
					ratioLabels := []string{"depth_ratio:bid", "depth_ratio:ask", "spread_ratio"}
					divergenceLabels := []string{"depth_divergence:bid", "depth_divergence:ask", "spread_divergence"}
					noiseLabels := []string{"depth_noise_scale:bid", "depth_noise_scale:ask", "spread_noise_scale"}
					zscoreLabels := []string{"depth_zscore:bid", "depth_zscore:ask", "spread_zscore"}

					for index, channel := range reading.Channels {
						if !channel.HasPrior {
							continue
						}
						m.WriteMetric(baselineLabels[index], channel.Baseline)
						m.WriteMetric(ratioLabels[index], originals[index]/channel.Baseline)
						m.WriteMetric(divergenceLabels[index], channel.Residual)

						if channel.ScoreScale > 0 {
							m.WriteMetric(noiseLabels[index], channel.ScoreScale)
							m.WriteStandardized(zscoreLabels[index], channel.ZScore)
						}
					}
				},
			),
			data.NewAdapter(
				statistic.NewLocalRegression(),
				func(m *data.Measurement[float64]) temporal.Price {
					return temporal.Price{At: m.At.UnixNano(), Value: m.GetMetric("depth_divergence:bid").Raw}
				},
				func(m *data.Measurement[float64], out statistic.LocalRegressionReading) {
					if out.SlopeDefined {
						m.WriteMetric("divergence_velocity:bid", out.Slope)
					}
					if out.SNRDefined {
						m.WriteMetric("divergence_velocity_snr:bid", out.SNR)
					}
				},
			),
			data.NewAdapter(
				statistic.NewLocalRegression(),
				func(m *data.Measurement[float64]) temporal.Price {
					return temporal.Price{At: m.At.UnixNano(), Value: m.GetMetric("depth_divergence:ask").Raw}
				},
				func(m *data.Measurement[float64], out statistic.LocalRegressionReading) {
					if out.SlopeDefined {
						m.WriteMetric("divergence_velocity:ask", out.Slope)
					}
					if out.SNRDefined {
						m.WriteMetric("divergence_velocity_snr:ask", out.SNR)
					}
				},
			),
			data.NewAdapter(
				statistic.NewLocalRegression(),
				func(m *data.Measurement[float64]) temporal.Price {
					return temporal.Price{At: m.At.UnixNano(), Value: m.GetMetric("spread_divergence").Raw}
				},
				func(m *data.Measurement[float64], out statistic.LocalRegressionReading) {
					if out.SlopeDefined {
						m.WriteMetric("spread_divergence_velocity", out.Slope)
					}
					if out.SNRDefined {
						m.WriteMetric("spread_divergence_velocity_snr", out.SNR)
					}
				},
			),
		),
		data.NewRecurrence(
			"depth_zscore:bid",
			"depth_zscore:ask",
			"spread_zscore",
		),
		data.NewFinalizer[float64](),
	)

	actual, _ := ticker.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

func (ticker *Ticker) Step(prior *data.Measurement[float64]) *data.Measurement[float64] {
	if ticker.Status() != runtime.READY {
		errnie.Warn(ticker.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		return nil
	}

	_, hasBid := prior.LookupMetric("bid")
	_, hasAsk := prior.LookupMetric("ask")
	_, hasBidQty := prior.LookupMetric("bid_qty")
	_, hasAskQty := prior.LookupMetric("ask_qty")

	if !hasBid || !hasAsk || !hasBidQty || !hasAskQty {
		return nil
	}

	out := ticker.arena.NewMeasurement(ticker.Source())
	out.Label = prior.Label
	out.SeqIdx = prior.SeqIdx
	out.At = prior.At
	out.From = prior.From
	out.Peers = []*data.Measurement[float64]{prior}

	if channel, hasCh := prior.GetProvenance("channel"); hasCh {
		out.SetProvenance("channel", channel)
	}

	res := data.Read[*data.Measurement[float64]](ticker.pipelineFor(out.Label).Next(
		transport.NewOne(unsafe.Pointer(&out)).Next(nil),
	))

	if res == nil {
		return out
	}

	return res
}
