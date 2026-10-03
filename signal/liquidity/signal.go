package liquidity

import (
	"context"
	"strconv"
	"sync"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/broker"
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	nmliquidity "github.com/theapemachine/symm/nomagique/liquidity"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

type Signal struct {
	*runtime.System
	arena     *data.ArenaOwner
	pipelines sync.Map
	ID        int
	books     broker.BookSource
}

func NewSignal(ctx context.Context, arena *data.ArenaOwner, books ...broker.BookSource) *Signal {
	signal := &Signal{
		arena: arena,
	}
	if len(books) > 0 {
		signal.books = books[0]
	}

	signal.System = runtime.NewSystem(ctx, "liquidity:signal", signal)
	return signal
}

func (signal *Signal) Arena() *data.ArenaOwner {
	return signal.arena
}

func (signal *Signal) pipelineFor(symbol string) core.Primitive {
	if existing, ok := signal.pipelines.Load(symbol); ok {
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

	actual, _ := signal.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

func (signal *Signal) Step(prior *data.Measurement[float64]) *data.Measurement[float64] {
	if signal.Status() != runtime.READY {
		errnie.Warn(signal.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		return nil
	}

	var bid, ask, bidQty, askQty float64
	if bMetric, ok := prior.LookupMetric("bid"); ok && bMetric.Raw > 0 {
		bid = bMetric.Raw
	}
	if aMetric, ok := prior.LookupMetric("ask"); ok && aMetric.Raw > 0 {
		ask = aMetric.Raw
	}
	if bqMetric, ok := prior.LookupMetric("bid_qty"); ok && bqMetric.Raw > 0 {
		bidQty = bqMetric.Raw
	}
	if aqMetric, ok := prior.LookupMetric("ask_qty"); ok && aqMetric.Raw > 0 {
		askQty = aqMetric.Raw
	}

	if (bid <= 0 || ask <= 0 || bidQty <= 0 || askQty <= 0) && signal.books != nil {
		signal.books.Book(prior.Label, func(b *spotbook.Book) {
			if b == nil {
				return
			}
			if bestBid := b.BestBid(); bestBid != nil && bestBid.Price != nil && bestBid.Quantity != nil {
				bid = bestBid.Price.Float64()
				bidQty = bestBid.Quantity.Float64()
			}
			if bestAsk := b.BestAsk(); bestAsk != nil && bestAsk.Price != nil && bestAsk.Quantity != nil {
				ask = bestAsk.Price.Float64()
				askQty = bestAsk.Quantity.Float64()
			}
		})
	}

	if bid <= 0 || ask <= 0 || bidQty <= 0 || askQty <= 0 {
		return nil
	}

	out := signal.arena.NewMeasurement(signal.Name())
	out.Label = prior.Label
	out.SeqIdx = prior.SeqIdx
	out.At = prior.At
	out.From = prior.From
	out.Peers = []*data.Measurement[float64]{prior}

	out.WriteMetric("bid", bid)
	out.WriteMetric("ask", ask)
	out.WriteMetric("bid_qty", bidQty)
	out.WriteMetric("ask_qty", askQty)

	if channel, hasCh := prior.GetProvenance("channel"); hasCh {
		out.SetProvenance("channel", channel)
	}

	res := data.Read[*data.Measurement[float64]](signal.pipelineFor(out.Label).Next(
		transport.NewOne(unsafe.Pointer(&out)).Next(nil),
	))

	if res == nil {
		return out
	}

	return res
}
