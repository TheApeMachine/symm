package morphology

import (
	"context"
	"strconv"
	"sync"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	nmmorphology "github.com/theapemachine/symm/nomagique/morphology"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Level3 is the book-morphology measuring instrument. It holds no state and no logic of
its own: its entire behavior is one nomagique pipeline over the measurement itself —
every stage writes its facts into the measurement where it computes them, and the
workload's register owns the measurement's lifetime.
*/
type Level3 struct {
	*runtime.System
	arena     *data.ArenaOwner
	pipelines sync.Map
	ID        int
	books     broker.BookSource
}

func NewLevel3(ctx context.Context, arena *data.ArenaOwner, books broker.BookSource) *Level3 {
	level3 := &Level3{
		arena: arena,
		books: books,
	}

	level3.System = runtime.NewSystem(ctx, "morphology:level3", level3)
	return level3
}

func (level3 *Level3) Source() string {
	return "morphology:level3"
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
		// 0. Extract raw morphology facts from order book
		nmmorphology.NewShapeFlow(level3.books),
		// 1. Adaptive baseline for morphology change
		transport.NewFan(
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("morphology_change").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(out.Count, 'f', -1, 64))

					if out.HasPrior {
						m.WriteMetric("morphology_change_baseline", out.Baseline)
						m.WriteStandardized("morphology_change_zscore", out.ZScore)
						m.SetMetadata(data.MetadataDivergence, strconv.FormatFloat(out.Residual, 'f', -1, 64))

						if out.VarianceDefined {
							m.SetMetadata(data.MetadataNoiseVariance, strconv.FormatFloat(out.Variance, 'f', -1, 64))
						}
					}
				},
			),
		),
		// 2. Recurrence on morphology geometry
		data.NewRecurrence(
			"book_shape_distance",
			"book_shape_ks",
			"morphology_change",
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
