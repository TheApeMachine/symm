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

				for out := range op.search.Next(transport.NewOne(unsafe.Pointer(&input)).Next(nil)) {
					op.reading = *(*nmcorrelation.LeadLagReading)(out)
				}

				if err := op.search.Error(); err != nil {
					op.Error(err)
					continue
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
