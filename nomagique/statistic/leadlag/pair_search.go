package leadlag

import (
	"iter"
	"sort"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	correlation "github.com/theapemachine/symm/nomagique/statistic/correlation"
)

/*
PairSearch searches the lag profile of the focal symbol against every
retained peer, selects the best defined pair (lexicographically last),
and stamps the pair's metrics onto the measurement. Arrivals with no
defined pair are dropped.
*/
type PairSearch struct {
	*core.PrimitiveError
	retained map[string]correlation.PathReading
	search   core.Primitive
	reading  correlation.LeadLagReading
}

func NewPairSearch(
	retained map[string]correlation.PathReading,
	estimator core.Primitive,
) *PairSearch {
	return &PairSearch{
		PrimitiveError: core.NewPrimitiveError(),
		retained:       retained,
		search:         correlation.NewLeadLag(estimator),
	}
}

func (pairSearch *PairSearch) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			focal, ok := pairSearch.retained[m.Label]

			if !ok {
				continue
			}

			symbols := make([]string, 0, len(pairSearch.retained))

			for symbol := range pairSearch.retained {
				if symbol != m.Label {
					symbols = append(symbols, symbol)
				}
			}

			sort.Strings(symbols)

			var selected correlation.LeadLagReading
			found := false

			for _, symbol := range symbols {
				peer := pairSearch.retained[symbol]
				input := correlation.LagProfileInput{Left: focal.Observations, Right: peer.Observations}

				for out := range pairSearch.search.Next(sequence.NewOne(unsafe.Pointer(&input)).Next(nil)) {
					pairSearch.reading = *(*correlation.LeadLagReading)(out)
				}

				if err := pairSearch.search.Error(); err != nil {
					pairSearch.Error(err)
					continue
				}

				if !pairSearch.reading.Defined {
					continue
				}

				selected = pairSearch.reading
				found = true
			}

			if !found {
				continue
			}

			resolution := selected.Spacing * 1e-9

			m.Metrics["contemporaneous_correlation"] = m.Metrics["contemporaneous_correlation"].Write(selected.Contemporaneous)
			m.Metrics["best_lag_correlation"] = m.Metrics["best_lag_correlation"].Write(selected.Correlation)
			m.Metrics["absolute_correlation_gain"] = m.Metrics["absolute_correlation_gain"].Write(selected.AbsoluteGain)
			m.Metrics["lag_fraction"] = m.Metrics["lag_fraction"].Write(selected.LagFraction)
			m.Metrics["best_lag_index"] = m.Metrics["best_lag_index"].Write(selected.LagIndex)
			m.Metrics["reference_return_count"] = m.Metrics["reference_return_count"].Write(selected.Observations)
			m.Metrics["measured_return_count"] = m.Metrics["measured_return_count"].Write(selected.Observations)
			m.Metrics["overlap_pair_count"] = m.Metrics["overlap_pair_count"].Write(selected.Support)
			m.Metrics["effective_sample_count"] = m.Metrics["effective_sample_count"].Write(selected.Support)
			m.Metrics["search_count"] = m.Metrics["search_count"].Write(selected.SearchCount)
			m.Metrics["best_lag_seconds"] = m.Metrics["best_lag_seconds"].Write(selected.X)
			m.Metrics["lag_search_resolution_seconds"] = m.Metrics["lag_search_resolution_seconds"].Write(resolution)
			m.Metrics["lag_search_span"] = m.Metrics["lag_search_span"].Write(selected.Span * resolution)

			if selected.ShapeDefined {
				m.Metrics["lag_peak_prominence"] = m.Metrics["lag_peak_prominence"].Write(selected.Prominence)
				m.Metrics["lag_peak_curvature"] = m.Metrics["lag_peak_curvature"].Write(selected.Curvature)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
