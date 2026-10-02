package pumpdump

import (
	"context"
	"math"
	"strconv"
	"sync"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	nmpumpdump "github.com/theapemachine/symm/nomagique/pumpdump"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Level3 is the authoritative executable-touch market entity. It holds no state
and no logic of its own: its entire behavior is one nomagique pipeline over the
measurement itself.
*/
type Level3 struct {
	*runtime.System
	arena     *data.ArenaOwner
	books     broker.BookSource
	pipelines sync.Map
	ID        int
}

func NewLevel3(ctx context.Context, arena *data.ArenaOwner, books broker.BookSource) *Level3 {
	level3 := &Level3{
		arena: arena,
		books: books,
	}

	level3.System = runtime.NewSystem(ctx, "pumpdump:level3", level3)
	return level3
}

func (level3 *Level3) Source() string {
	return "pumpdump:level3"
}

func (level3 *Level3) Arena() *data.ArenaOwner {
	return level3.arena
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/

func (level3 *Level3) pipelineFor(symbol string) core.Primitive {
	if existing, ok := level3.pipelines.Load(symbol); ok {
		return existing.(core.Primitive)
	}

	pipeline := nomagique.NewNumber(
		// 0. Extract and validate book touch geometry
		nmpumpdump.NewBookTouch(level3.books),

		// 1. Adaptive baseline and dynamics for relative spread
		transport.NewFan(
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("relative_spread").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					m.WriteMetric("relative_spread_baseline", out.Baseline)
					m.EnsureMetadata()
					m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(out.Count, 'f', -1, 64))

					if out.HasPrior {
						if out.Baseline > 0 {
							rs := m.GetMetric("relative_spread").Raw
							spreadRatio := rs / out.Baseline
							m.WriteMetric("spread_ratio", spreadRatio)
							if rs > 0 {
								divergence := math.Log(spreadRatio)
								m.WriteMetric("spread_divergence", divergence)
								m.SetMetadata(data.MetadataDivergence, strconv.FormatFloat(divergence, 'f', -1, 64))
							}
						}

						m.WriteStandardized("spread_zscore", out.ZScore)
						if out.VarianceDefined {
							m.SetMetadata(data.MetadataNoiseVariance, strconv.FormatFloat(out.Variance, 'f', -1, 64))
						}
					}
				},
			),
			data.NewAdapter(
				temporal.NewVelocity(),
				func(m *data.Measurement[float64]) temporal.Observation {
					if div, ok := m.LookupMetric("spread_divergence"); ok {
						return temporal.Observation{Value: div.Raw, At: m.At.UnixNano()}
					}
					return temporal.Observation{Value: 0, At: m.At.UnixNano()}
				},
				func(m *data.Measurement[float64], out temporal.VelocityReading) {
					if out.Defined {
						m.WriteMetric("spread_divergence_velocity", out.Rate)
					}
				},
			),
		),

		// 2. Recurrence on spread dynamics
		data.NewRecurrence(
			"spread",
			"relative_spread",
			"spread_divergence",
		),

		// 3. Finalize
		data.NewFinalizer[float64](),
	)

	actual, _ := level3.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

func (level3 *Level3) Step(prior *data.Measurement[float64]) *data.Measurement[float64] {
	if level3.Status() != runtime.READY {
		errnie.Warn(level3.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		return nil
	}

	out := level3.arena.NewMeasurement(level3.Source())
	out.Label = prior.Label
	out.SeqIdx = prior.SeqIdx
	out.At = prior.At
	out.From = prior.From
	out.Peers = []*data.Measurement[float64]{prior}

	if channel, hasCh := prior.GetProvenance("channel"); hasCh {
		out.SetProvenance("channel", channel)
	}

	res := data.Read[*data.Measurement[float64]](level3.pipelineFor(out.Label).Next(
		transport.NewOne(unsafe.Pointer(&out)).Next(nil),
	))

	if res == nil {
		return out
	}

	return res
}
