package depthflow

import (
	"context"
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

type depthInput struct {
	ObservedBid float64
	ObservedAsk float64
	MutationBid float64
	MutationAsk float64
	At          time.Time
}

type depthResult struct {
	Observed                  float64
	ObservedDiff              float64
	ObservedImbalance         float64
	MutationCount             float64
	MutationCountDiff         float64
	MutationActivityImbalance float64
	Rate                      float64
	HasRate                   bool
	ImbalanceReading          adaptive.BaselineReading
	RateReading               adaptive.BaselineReading
}

type depthPipeline struct {
	*core.PrimitiveError
	hasPrev   bool
	prevTime  time.Time
	imbalance core.Primitive
	rate      core.Primitive
	out       depthResult
}

func newDepthPipeline() core.Primitive {
	return &depthPipeline{
		PrimitiveError: core.NewPrimitiveError(),
		imbalance:      adaptive.NewBaseline(adaptive.NewWindow()),
		rate:           adaptive.NewBaseline(adaptive.NewWindow()),
	}
}

func (op *depthPipeline) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*depthInput)(arriving)

			observed := input.ObservedBid + input.ObservedAsk
			observedDiff := input.ObservedBid - input.ObservedAsk
			mutations := input.MutationBid + input.MutationAsk
			mutationDiff := input.MutationBid - input.MutationAsk

			op.out = depthResult{
				Observed:          observed,
				ObservedDiff:      observedDiff,
				MutationCount:     mutations,
				MutationCountDiff: mutationDiff,
			}

			if observed > 0 {
				imbalance := observedDiff / observed
				op.out.ObservedImbalance = imbalance

				for rPtr := range op.imbalance.Next(transport.NewOne(unsafe.Pointer(&imbalance)).Next(nil)) {
					op.out.ImbalanceReading = *(*adaptive.BaselineReading)(rPtr)
				}
			}

			if mutations > 0 {
				op.out.MutationActivityImbalance = mutationDiff / mutations
			}

			if op.hasPrev {
				dt := input.At.Sub(op.prevTime).Seconds()

				if dt > 0 {
					rate := observed / dt
					op.out.Rate = rate
					op.out.HasRate = true

					for rPtr := range op.rate.Next(transport.NewOne(unsafe.Pointer(&rate)).Next(nil)) {
						op.out.RateReading = *(*adaptive.BaselineReading)(rPtr)
					}
				}
			}

			op.prevTime = input.At
			op.hasPrev = true

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Level3 is the depth-flow measuring instrument. It holds no state and no logic of
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
		System:   runtime.NewSystem(ctx, "depthflow:level3"),
		pipeline: nomagique.NewNumber(newDepthPipeline()),
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

	input := depthInput{
		ObservedBid: m.Metrics["observed_notional:bid"].Raw,
		ObservedAsk: m.Metrics["observed_notional:ask"].Raw,
		MutationBid: m.Metrics["mutation_count:bid"].Raw,
		MutationAsk: m.Metrics["mutation_count:ask"].Raw,
		At:          m.At,
	}

	for out := range level3.pipeline.Next(transport.NewOne(unsafe.Pointer(&input)).Next(nil)) {
		res := (*depthResult)(out)

		m.Metrics["observed_notional"] = m.Metrics["observed_notional"].Write(res.Observed)
		m.Metrics["observed_notional_diff"] = m.Metrics["observed_notional_diff"].Write(res.ObservedDiff)
		m.Metrics["mutation_count"] = m.Metrics["mutation_count"].Write(res.MutationCount)
		m.Metrics["mutation_count_diff"] = m.Metrics["mutation_count_diff"].Write(res.MutationCountDiff)

		if res.Observed > 0 {
			m.Metrics["observed_notional_imbalance"] = m.Metrics["observed_notional_imbalance"].Write(res.ObservedImbalance)
			m.Metadata[data.MetadataSupport] = res.ImbalanceReading.Count

			if res.ImbalanceReading.HasPrior {
				m.Metrics["observed_notional_imbalance_baseline"] = m.Metrics["observed_notional_imbalance_baseline"].Write(res.ImbalanceReading.Baseline)
				m.Metrics["observed_notional_imbalance_divergence"] = m.Metrics["observed_notional_imbalance_divergence"].Write(res.ImbalanceReading.Residual)
				m.Metrics["observed_notional_imbalance_zscore"] = m.Metrics["observed_notional_imbalance_zscore"].Write(res.ImbalanceReading.ZScore)
				m.Metadata[data.MetadataDivergence] = res.ImbalanceReading.Residual

				if res.ImbalanceReading.VarianceDefined {
					m.Metadata[data.MetadataNoiseVariance] = res.ImbalanceReading.Variance
				}
			}
		}

		if res.MutationCount > 0 {
			m.Metrics["mutation_activity_imbalance"] = m.Metrics["mutation_activity_imbalance"].Write(res.MutationActivityImbalance)
		}

		if res.HasRate {
			m.Metrics["observed_notional_rate"] = m.Metrics["observed_notional_rate"].Write(res.Rate)

			if res.RateReading.HasPrior {
				m.Metrics["observed_notional_rate_baseline"] = m.Metrics["observed_notional_rate_baseline"].Write(res.RateReading.Baseline)
				m.Metrics["observed_notional_rate_divergence"] = m.Metrics["observed_notional_rate_divergence"].Write(res.RateReading.Residual)
				m.Metrics["observed_notional_rate_zscore"] = m.Metrics["observed_notional_rate_zscore"].Write(res.RateReading.ZScore)
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
	return data.NewMeasurement[float64]("depthflow:level3", map[string]data.Metric[float64]{
		"observed_notional:bid":                 data.NewMetric[float64]("observed_notional:bid", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"observed_notional:ask":                 data.NewMetric[float64]("observed_notional:ask", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"observed_notional":                     data.NewMetric[float64]("observed_notional", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"observed_notional_diff":                data.NewMetric[float64]("observed_notional_diff", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"add_notional:bid":                      data.NewMetric[float64]("add_notional:bid", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"add_notional:ask":                      data.NewMetric[float64]("add_notional:ask", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"modify_remaining_notional:bid":         data.NewMetric[float64]("modify_remaining_notional:bid", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"modify_remaining_notional:ask":         data.NewMetric[float64]("modify_remaining_notional:ask", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"delete_count:bid":                      data.NewMetric[float64]("delete_count:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"delete_count:ask":                      data.NewMetric[float64]("delete_count:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"mutation_count:bid":                    data.NewMetric[float64]("mutation_count:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"mutation_count:ask":                    data.NewMetric[float64]("mutation_count:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"mutation_count":                        data.NewMetric[float64]("mutation_count", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"mutation_count_diff":                   data.NewMetric[float64]("mutation_count_diff", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"mutation_activity_imbalance":           data.NewMetric[float64]("mutation_activity_imbalance", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"observed_notional_imbalance":           data.NewMetric[float64]("observed_notional_imbalance", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"observed_notional_rate":                data.NewMetric[float64]("observed_notional_rate", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"observed_notional_imbalance_baseline":   data.NewMetric[float64]("observed_notional_imbalance_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"observed_notional_imbalance_divergence": data.NewMetric[float64]("observed_notional_imbalance_divergence", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"observed_notional_imbalance_zscore":     data.NewMetric[float64]("observed_notional_imbalance_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"observed_notional_rate_baseline":       data.NewMetric[float64]("observed_notional_rate_baseline", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"observed_notional_rate_divergence":     data.NewMetric[float64]("observed_notional_rate_divergence", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"observed_notional_rate_zscore":         data.NewMetric[float64]("observed_notional_rate_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
	})
}
