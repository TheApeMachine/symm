package leadlag

import (
	"iter"
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
			m := *(**data.Measurement[float64])(arriving)

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

			m.WriteMetric("contemporaneous_correlation", selected.Contemporaneous)
			m.WriteMetric("best_lag_correlation", selected.Correlation)
			m.WriteMetric("absolute_correlation_gain", selected.AbsoluteGain)
			m.WriteMetric("lag_fraction", selected.LagFraction)
			m.WriteMetric("best_lag_index", selected.LagIndex)
			m.WriteMetric("reference_return_count", selected.Observations)
			m.WriteMetric("measured_return_count", selected.Observations)
			m.WriteMetric("overlap_pair_count", selected.Support)
			m.WriteMetric("effective_sample_count", selected.Support)
			m.WriteMetric("search_count", selected.SearchCount)
			m.WriteMetric("best_lag_seconds", selected.X)
			m.WriteMetric("lag_search_resolution_seconds", resolution)
			m.WriteMetric("lag_search_span", selected.Span * resolution)

			if selected.ShapeDefined {
				m.WriteMetric("lag_peak_prominence", selected.Prominence)
				m.WriteMetric("lag_peak_curvature", selected.Curvature)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
