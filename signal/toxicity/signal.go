package toxicity

import (
	"context"
	"strconv"
	"sync"
	"unsafe"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtoxicity "github.com/theapemachine/symm/nomagique/toxicity"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Signal is the unified toxic-flow and trade-matching measuring instrument.
It captures book touch dispositions (retreats, withdrawals, replenishments)
and trade-matching fill dynamics, outputting exactly one measurement per trade tick.
*/
type Signal struct {
	*runtime.System
	arena     *data.ArenaOwner
	books     broker.BookSource
	pipelines sync.Map
	ID        int
}

func NewSignal(ctx context.Context, arena *data.ArenaOwner, books ...broker.BookSource) *Signal {
	signal := &Signal{
		arena: arena,
	}
	if len(books) > 0 {
		signal.books = books[0]
	}

	signal.System = runtime.NewSystem(ctx, "toxicity", signal)
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
		// 0. Extract raw depth facts and compute touch disposition
		nmtoxicity.NewTouchDisposition(),

		// 1. Match trades against touch quotes
		nmtoxicity.NewTradeMatching(),

		// 2. Baselines and dynamics
		transport.NewFan(
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("net_withdrawal_fraction:bid").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.SetMetric("withdrawal_fraction_baseline:bid", data.NewMetric[float64](
							"withdrawal_fraction_baseline:bid",
							data.UnitDimensionless,
							data.TimescaleInstantaneous,
							out.Baseline,
							out.ScoreScale,
						).Write(out.Baseline))
						m.SetMetric("withdrawal_fraction_divergence:bid", data.NewMetric[float64](
							"withdrawal_fraction_divergence:bid",
							data.UnitDimensionless,
							data.TimescaleInstantaneous,
							0.0,
							out.ScoreScale,
						).Write(out.Residual))
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
						m.SetMetric("withdrawal_fraction_baseline:ask", data.NewMetric[float64](
							"withdrawal_fraction_baseline:ask",
							data.UnitDimensionless,
							data.TimescaleInstantaneous,
							out.Baseline,
							out.ScoreScale,
						).Write(out.Baseline))
						m.SetMetric("withdrawal_fraction_divergence:ask", data.NewMetric[float64](
							"withdrawal_fraction_divergence:ask",
							data.UnitDimensionless,
							data.TimescaleInstantaneous,
							0.0,
							out.ScoreScale,
						).Write(out.Residual))
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
						m.SetMetric("retreat_fraction_baseline:bid", data.NewMetric[float64](
							"retreat_fraction_baseline:bid",
							data.UnitDimensionless,
							data.TimescaleInstantaneous,
							out.Baseline,
							out.ScoreScale,
						).Write(out.Baseline))
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
						m.SetMetric("retreat_fraction_baseline:ask", data.NewMetric[float64](
							"retreat_fraction_baseline:ask",
							data.UnitDimensionless,
							data.TimescaleInstantaneous,
							out.Baseline,
							out.ScoreScale,
						).Write(out.Baseline))
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
						m.SetMetric("replenishment_fraction_baseline:bid", data.NewMetric[float64](
							"replenishment_fraction_baseline:bid",
							data.UnitDimensionless,
							data.TimescaleInstantaneous,
							out.Baseline,
							out.ScoreScale,
						).Write(out.Baseline))
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
						m.SetMetric("replenishment_fraction_baseline:ask", data.NewMetric[float64](
							"replenishment_fraction_baseline:ask",
							data.UnitDimensionless,
							data.TimescaleInstantaneous,
							out.Baseline,
							out.ScoreScale,
						).Write(out.Baseline))
					}
				},
			),
			data.NewAdapter(
				temporal.NewVelocity(),
				func(m *data.Measurement[float64]) temporal.Observation {
					if v, ok := m.LookupMetric("net_withdrawal_fraction:bid"); ok {
						return temporal.Observation{Value: v.Raw, At: m.At.UnixNano()}
					}
					return temporal.Observation{Value: 0, At: m.At.UnixNano()}
				},
				func(m *data.Measurement[float64], out temporal.VelocityReading) {
					if out.Defined {
						m.SetMetric("withdrawal_fraction_velocity:bid", data.NewMetric[float64](
							"withdrawal_fraction_velocity:bid",
							data.UnitVelocity,
							data.TimescaleInstantaneous,
							0.0,
							0.0,
						).Write(out.Rate))
					}
				},
			),
			data.NewAdapter(
				temporal.NewVelocity(),
				func(m *data.Measurement[float64]) temporal.Observation {
					if v, ok := m.LookupMetric("net_withdrawal_fraction:ask"); ok {
						return temporal.Observation{Value: v.Raw, At: m.At.UnixNano()}
					}
					return temporal.Observation{Value: 0, At: m.At.UnixNano()}
				},
				func(m *data.Measurement[float64], out temporal.VelocityReading) {
					if out.Defined {
						m.SetMetric("withdrawal_fraction_velocity:ask", data.NewMetric[float64](
							"withdrawal_fraction_velocity:ask",
							data.UnitVelocity,
							data.TimescaleInstantaneous,
							0.0,
							0.0,
						).Write(out.Rate))
					}
				},
			),
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("touch_fill_fraction:bid").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.SetMetric("fill_fraction_baseline:bid", data.NewMetric[float64](
							"fill_fraction_baseline:bid",
							data.UnitDimensionless,
							data.TimescaleInstantaneous,
							out.Baseline,
							out.ScoreScale,
						).Write(out.Baseline))
						m.SetMetric("fill_fraction_divergence:bid", data.NewMetric[float64](
							"fill_fraction_divergence:bid",
							data.UnitDimensionless,
							data.TimescaleInstantaneous,
							0.0,
							out.ScoreScale,
						).Write(out.Residual))
						m.WriteStandardized("fill_fraction_zscore:bid", out.ZScore)
					}
					m.EnsureMetadata()
					m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(out.Count, 'f', -1, 64))
					if out.VarianceDefined {
						m.SetMetadata(data.MetadataNoiseVariance, strconv.FormatFloat(out.Variance, 'f', -1, 64))
					}
				},
			),
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("touch_fill_fraction:ask").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.SetMetric("fill_fraction_baseline:ask", data.NewMetric[float64](
							"fill_fraction_baseline:ask",
							data.UnitDimensionless,
							data.TimescaleInstantaneous,
							out.Baseline,
							out.ScoreScale,
						).Write(out.Baseline))
						m.SetMetric("fill_fraction_divergence:ask", data.NewMetric[float64](
							"fill_fraction_divergence:ask",
							data.UnitDimensionless,
							data.TimescaleInstantaneous,
							0.0,
							out.ScoreScale,
						).Write(out.Residual))
						m.WriteStandardized("fill_fraction_zscore:ask", out.ZScore)
					}
				},
			),
			data.NewAdapter(
				temporal.NewVelocity(),
				func(m *data.Measurement[float64]) temporal.Observation {
					if v, ok := m.LookupMetric("touch_fill_fraction:bid"); ok {
						return temporal.Observation{Value: v.Raw, At: m.At.UnixNano()}
					}
					return temporal.Observation{Value: 0, At: m.At.UnixNano()}
				},
				func(m *data.Measurement[float64], out temporal.VelocityReading) {
					if out.Defined {
						m.SetMetric("fill_fraction_velocity:bid", data.NewMetric[float64](
							"fill_fraction_velocity:bid",
							data.UnitVelocity,
							data.TimescaleInstantaneous,
							0.0,
							0.0,
						).Write(out.Rate))
					}
				},
			),
			data.NewAdapter(
				temporal.NewVelocity(),
				func(m *data.Measurement[float64]) temporal.Observation {
					if v, ok := m.LookupMetric("touch_fill_fraction:ask"); ok {
						return temporal.Observation{Value: v.Raw, At: m.At.UnixNano()}
					}
					return temporal.Observation{Value: 0, At: m.At.UnixNano()}
				},
				func(m *data.Measurement[float64], out temporal.VelocityReading) {
					if out.Defined {
						m.SetMetric("fill_fraction_velocity:ask", data.NewMetric[float64](
							"fill_fraction_velocity:ask",
							data.UnitVelocity,
							data.TimescaleInstantaneous,
							0.0,
							0.0,
						).Write(out.Rate))
					}
				},
			),
		),

		// 3. Recurrence
		data.NewRecurrence(
			"net_withdrawal_fraction:bid",
			"net_withdrawal_fraction:ask",
			"retreat_fraction:bid",
			"retreat_fraction:ask",
			"touch_fill_fraction:bid",
			"touch_fill_fraction:ask",
		),

		// 4. Finalize
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

	out := signal.arena.NewMeasurement(signal.Name())
	out.Epoch = prior.Epoch
	out.Tick = prior.Tick
	out.Label = prior.Label
	out.SeqIdx = prior.SeqIdx
	out.At = prior.At
	out.From = prior.From
	out.Peers = []*data.Measurement[float64]{prior}

	if side, hasSide := prior.GetProvenance("side"); hasSide {
		out.SetProvenance("side", side)
	}
	if channel, hasCh := prior.GetProvenance("channel"); hasCh {
		out.SetProvenance("channel", channel)
	}

	// Copy trade metrics from prior
	if price, ok := prior.LookupMetric("price"); ok {
		out.SetMetric("price", price)
	}
	if qty, ok := prior.LookupMetric("qty"); ok {
		out.SetMetric("qty", qty)
	}

	// Copy book metrics from prior if present
	if bestBid, ok := prior.LookupMetric("best_bid"); ok {
		out.SetMetric("best_bid", bestBid)
	}
	if bestAsk, ok := prior.LookupMetric("best_ask"); ok {
		out.SetMetric("best_ask", bestAsk)
	}
	if touchQtyBid, ok := prior.LookupMetric("touch_quantity:bid"); ok {
		out.SetMetric("touch_quantity:bid", touchQtyBid)
	}
	if touchQtyAsk, ok := prior.LookupMetric("touch_quantity:ask"); ok {
		out.SetMetric("touch_quantity:ask", touchQtyAsk)
	}

	// If book touch not in prior, query books directly
	if signal.books != nil {
		signal.books.Book(out.Label, func(b *spotbook.Book) {
			if b == nil {
				return
			}
			bestBid := b.BestBid()
			bestAsk := b.BestAsk()
			if bestBid != nil && bestBid.Price != nil && bestBid.Quantity != nil &&
				bestAsk != nil && bestAsk.Price != nil && bestAsk.Quantity != nil {
				bidPrice := bestBid.Price.Float64()
				askPrice := bestAsk.Price.Float64()
				bidQty := bestBid.Quantity.Float64()
				askQty := bestAsk.Quantity.Float64()
				midpoint := (bidPrice + askPrice) / 2.0
				spread := askPrice - bidPrice
				totalQty := bidQty + askQty

				if _, ok := out.LookupMetric("best_bid"); !ok {
					out.SetMetric("best_bid", data.NewMetric[float64](
						"best_bid",
						data.UnitPrice,
						data.TimescaleInstantaneous,
						midpoint,
						spread,
					).Write(bidPrice))
				}
				if _, ok := out.LookupMetric("touch_quantity:bid"); !ok {
					out.SetMetric("touch_quantity:bid", data.NewMetric[float64](
						"touch_quantity:bid",
						data.UnitQuantity,
						data.TimescaleInstantaneous,
						0.0,
						totalQty,
					).Write(bidQty))
				}
				if _, ok := out.LookupMetric("best_ask"); !ok {
					out.SetMetric("best_ask", data.NewMetric[float64](
						"best_ask",
						data.UnitPrice,
						data.TimescaleInstantaneous,
						midpoint,
						spread,
					).Write(askPrice))
				}
				if _, ok := out.LookupMetric("touch_quantity:ask"); !ok {
					out.SetMetric("touch_quantity:ask", data.NewMetric[float64](
						"touch_quantity:ask",
						data.UnitQuantity,
						data.TimescaleInstantaneous,
						0.0,
						totalQty,
					).Write(askQty))
				}
			}
		})
	}

	res := data.Read[*data.Measurement[float64]](signal.pipelineFor(out.Label).Next(
		transport.NewOne(unsafe.Pointer(&out)).Next(nil),
	))

	if res == nil {
		return out
	}

	res.Finalize()
	return res
}
