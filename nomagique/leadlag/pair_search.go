package leadlag

import (
	"iter"
	"math"
	"sort"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
PairSearch searches the lag profile of the focal symbol against every
retained peer, selects the best defined pair (lexicographically last),
and stamps the pair's metrics onto the measurement. Arrivals with no
defined pair are dropped.
*/
type PairSearch struct {
	*core.PrimitiveError
	retained map[string]nmcorrelation.PathReading
	search   core.Primitive
	reading  nmcorrelation.LeadLagReading
}

func NewPairSearch(
	retained map[string]nmcorrelation.PathReading,
	estimator core.Primitive,
) core.Primitive {
	return &PairSearch{
		PrimitiveError: core.NewPrimitiveError(),
		retained:       retained,
		search:         nmcorrelation.NewLeadLag(estimator),
	}
}

func (op *PairSearch) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement)(arriving)

			focal, ok := op.retained[m.Label]

			if !ok {
				continue
			}

			symbols := make([]string, 0, len(op.retained))

			for symbol := range op.retained {
				if symbol != m.Label {
					symbols = append(symbols, symbol)
				}
			}

			sort.Strings(symbols)

			var selected nmcorrelation.LeadLagReading
			found := false

			for _, symbol := range symbols {
				peer := op.retained[symbol]
				input := nmcorrelation.LagProfileInput{Left: focal.Observations, Right: peer.Observations}

				if fast, ok := op.search.(interface {
					Search(*nmcorrelation.LagProfileInput) (nmcorrelation.LeadLagReading, error)
				}); ok {
					var err error
					op.reading, err = fast.Search(&input)

					if err != nil {
						op.Error(err)
						continue
					}
				} else {
					for out := range op.search.Next(transport.NewOne(unsafe.Pointer(&input)).Next(nil)) {
						op.reading = *(*nmcorrelation.LeadLagReading)(out)
					}

					if err := op.search.Error(); err != nil {
						op.Error(err)
						continue
					}
				}

				if !op.reading.Defined {
					continue
				}

				selected = op.reading
				found = true
			}

			if !found {
				continue
			}

			resolution := selected.Spacing * 1e-9

			spanScale := math.Max(selected.Span*resolution, 1e-6)

			m.SetMetric("contemporaneous_correlation", data.NewMetric(
				"contemporaneous_correlation",
				data.UnitCorrelation,
				data.TimescaleInstantaneous,
				0,
				1,
			).Write(selected.Contemporaneous))
			m.SetMetric("best_lag_correlation", data.NewMetric(
				"best_lag_correlation",
				data.UnitCorrelation,
				data.TimescaleInstantaneous,
				0,
				1,
			).Write(selected.Correlation))
			m.SetMetric("absolute_correlation_gain", data.NewMetric(
				"absolute_correlation_gain",
				data.UnitCorrelation,
				data.TimescaleInstantaneous,
				0,
				1,
			).Write(selected.AbsoluteGain))
			m.SetMetric("lag_fraction", data.NewMetric(
				"lag_fraction",
				data.UnitRatio,
				data.TimescaleRollingWindow,
				0,
				1,
			).Write(selected.LagFraction))
			m.SetMetric("best_lag_index", data.NewMetric(
				"best_lag_index",
				data.UnitCount,
				data.TimescaleInstantaneous,
				0,
				math.Max(float64(selected.SearchCount), 1),
			).Write(selected.LagIndex))
			m.SetMetric("reference_return_count", data.NewMetric(
				"reference_return_count",
				data.UnitCount,
				data.TimescaleRollingWindow,
				0,
				math.Max(float64(selected.Observations), 1),
			).Write(selected.Observations))
			m.SetMetric("measured_return_count", data.NewMetric(
				"measured_return_count",
				data.UnitCount,
				data.TimescaleRollingWindow,
				0,
				math.Max(float64(selected.Observations), 1),
			).Write(selected.Observations))
			m.SetMetric("overlap_pair_count", data.NewMetric(
				"overlap_pair_count",
				data.UnitCount,
				data.TimescaleRollingWindow,
				0,
				math.Max(float64(selected.Support), 1),
			).Write(selected.Support))
			m.SetMetric("effective_sample_count", data.NewMetric(
				"effective_sample_count",
				data.UnitCount,
				data.TimescaleRollingWindow,
				0,
				math.Max(float64(selected.Support), 1),
			).Write(selected.Support))
			m.SetMetric("search_count", data.NewMetric(
				"search_count",
				data.UnitCount,
				data.TimescaleRollingWindow,
				0,
				math.Max(float64(selected.SearchCount), 1),
			).Write(selected.SearchCount))
			m.SetMetric("best_lag_seconds", data.NewMetric(
				"best_lag_seconds",
				data.UnitDuration,
				data.TimescaleInstantaneous,
				0,
				spanScale,
			).Write(selected.X))
			m.SetMetric("lag_search_resolution_seconds", data.NewMetric(
				"lag_search_resolution_seconds",
				data.UnitDuration,
				data.TimescaleRollingWindow,
				resolution,
				resolution,
			).Write(resolution))
			m.SetMetric("lag_search_span", data.NewMetric(
				"lag_search_span",
				data.UnitDuration,
				data.TimescaleRollingWindow,
				0,
				spanScale,
			).Write(selected.Span*resolution))

			if selected.ShapeDefined {
				m.SetMetric("lag_peak_prominence", data.NewMetric(
					"lag_peak_prominence",
					data.UnitCorrelation,
					data.TimescaleInstantaneous,
					0,
					1,
				).Write(selected.Prominence))
				m.SetMetric("lag_peak_curvature", data.NewMetric(
					"lag_peak_curvature",
					data.UnitDimensionless,
					data.TimescaleInstantaneous,
					0,
					1,
				).Write(selected.Curvature))
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
