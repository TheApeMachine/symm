package morphology

import (
	"context"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

type morphologyInput struct {
	Distance float64
	KS       float64
	ConcBid  float64
	ConcAsk  float64
	EntBid   float64
	EntAsk   float64
}

type morphologyResult struct {
	Distance float64
	KS       float64
	ConcBid  float64
	ConcAsk  float64
	EntBid   float64
	EntAsk   float64
	HasPrev  bool
	Change   float64
	Reading  adaptive.BaselineReading
}

type morphologyPipeline struct {
	*core.PrimitiveError
	hasPrev      bool
	prevDistance float64
	baseline     core.Primitive
	out          morphologyResult
}

func newMorphologyPipeline() core.Primitive {
	return &morphologyPipeline{
		PrimitiveError: core.NewPrimitiveError(),
		baseline:       adaptive.NewBaseline(adaptive.NewWindow()),
	}
}

func (op *morphologyPipeline) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*morphologyInput)(arriving)

			op.out = morphologyResult{
				Distance: input.Distance,
				KS:       input.KS,
				ConcBid:  input.ConcBid,
				ConcAsk:  input.ConcAsk,
				EntBid:   input.EntBid,
				EntAsk:   input.EntAsk,
			}

			if op.hasPrev {
				change := math.Abs(input.Distance - op.prevDistance)
				op.out.HasPrev = true
				op.out.Change = change

				for rPtr := range op.baseline.Next(transport.NewOne(unsafe.Pointer(&change)).Next(nil)) {
					op.out.Reading = *(*adaptive.BaselineReading)(rPtr)
				}
			}

			op.prevDistance = input.Distance
			op.hasPrev = true

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Level3 is the book-morphology measuring instrument. It holds no state and no logic of
its own: its entire behavior is one nomagique pipeline over the measurement itself —
every stage writes its facts into the measurement where it computes them, and the
workload's register owns the measurement's lifetime.
*/
type Level3 struct {
	*runtime.System
	pipeline core.Primitive
	ID       int
}

func NewLevel3(ctx context.Context) *Level3 {
	return &Level3{
		System:   runtime.NewSystem(ctx, "morphology:level3"),
		pipeline: nomagique.NewNumber(newMorphologyPipeline()),
	}
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/
func (level3 *Level3) Step(m *data.Measurement[float64]) *data.Measurement[float64] {
	if m == nil {
		return nil
	}

	if m.Err != nil {
		return m
	}

	if m.Metadata == nil {
		m.Metadata = make(map[string]float64)
	}

	input := morphologyInput{
		Distance: m.Metrics["book_shape_distance"].Raw,
		KS:       m.Metrics["book_shape_ks"].Raw,
		ConcBid:  m.Metrics["concentration:bid"].Raw,
		ConcAsk:  m.Metrics["concentration:ask"].Raw,
		EntBid:   m.Metrics["entropy:bid"].Raw,
		EntAsk:   m.Metrics["entropy:ask"].Raw,
	}

	for out := range level3.pipeline.Next(transport.NewOne(unsafe.Pointer(&input)).Next(nil)) {
		res := (*morphologyResult)(out)

		m.Metrics["book_shape_distance"] = m.Metrics["book_shape_distance"].Write(res.Distance)
		m.Metrics["book_shape_ks"] = m.Metrics["book_shape_ks"].Write(res.KS)
		m.Metrics["concentration:bid"] = m.Metrics["concentration:bid"].Write(res.ConcBid)
		m.Metrics["concentration:ask"] = m.Metrics["concentration:ask"].Write(res.ConcAsk)
		m.Metrics["entropy:bid"] = m.Metrics["entropy:bid"].Write(res.EntBid)
		m.Metrics["entropy:ask"] = m.Metrics["entropy:ask"].Write(res.EntAsk)

		if res.HasPrev {
			m.Metrics["morphology_change"] = m.Metrics["morphology_change"].Write(res.Change)
			m.Metadata[data.MetadataSupport] = res.Reading.Count

			if res.Reading.HasPrior {
				m.Metrics["morphology_change_baseline"] = m.Metrics["morphology_change_baseline"].Write(res.Reading.Baseline)
				m.Metrics["morphology_change_zscore"] = m.Metrics["morphology_change_zscore"].Write(res.Reading.ZScore)
				m.Metadata[data.MetadataDivergence] = res.Reading.Residual

				if res.Reading.VarianceDefined {
					m.Metadata[data.MetadataNoiseVariance] = res.Reading.Variance
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
func (level3 *Level3) Register() *data.Measurement[float64] {
	return data.NewMeasurement[float64]("morphology:level3", map[string]data.Metric[float64]{
		"book_shape_distance":        data.NewMetric[float64]("book_shape_distance", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"book_shape_ks":              data.NewMetric[float64]("book_shape_ks", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"concentration:bid":          data.NewMetric[float64]("concentration:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"concentration:ask":          data.NewMetric[float64]("concentration:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"entropy:bid":                data.NewMetric[float64]("entropy:bid", data.UnitNat, data.TimescaleInstantaneous, 0, 1),
		"entropy:ask":                data.NewMetric[float64]("entropy:ask", data.UnitNat, data.TimescaleInstantaneous, 0, 1),
		"morphology_change":          data.NewMetric[float64]("morphology_change", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"morphology_change_baseline": data.NewMetric[float64]("morphology_change_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"morphology_change_zscore":   data.NewMetric[float64]("morphology_change_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
	})
}
