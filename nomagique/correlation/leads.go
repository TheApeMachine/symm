package correlation

import (
	"iter"
	"math"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Leads measures the focal symbol's retained price path against every peer path
held in the shared path store (see Member) across the exact discrete lag
search, without peers ever living on the Measurement. The focal path is the
measured path Y and each peer is the reference path X, so a positive lag means
changes in the peer precede corresponding changes in the focal symbol. Per pair
it composes LeadLag around the supplied asynchronous estimator and a per-peer
FisherEstimator for the best-lag correlation's causal history.

Each arrival is the *data.Adapter. Pair facts are published into the adapter
under "<fact>@<peer>"; an undefined search publishes nothing for that pair.
The adapter is yielded unchanged.
*/
type Leads struct {
	*core.PrimitiveError
	label   string
	paths   core.Primitive
	leadlag core.Primitive
	fisher  map[string]core.Primitive
	request map[string][][2]float64
}

func NewLeads(label string, paths core.Primitive, estimator core.Primitive) core.Primitive {
	return &Leads{
		PrimitiveError: core.NewPrimitiveError(),
		label:          label,
		paths:          paths,
		leadlag:        NewLeadLag(estimator),
		fisher:         make(map[string]core.Primitive),
		request:        make(map[string][][2]float64),
	}
}

func (op *Leads) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}
			if !yield(arriving) {
				return
			}
		}

		var held map[string][][2]float64
		for pointer := range op.paths.Next(data.NewValue(unsafe.Pointer(&op.request)).Next(nil)) {
			held = *(*map[string][][2]float64)(pointer)
		}

		if err := op.paths.Error(); err != nil {
			op.Error(err)
			return
		}

		measured, ok := held[op.label]
		if !ok {
			op.Error(core.ErrNotHeld)
			return
		}

		peers := make([]string, 0, len(held))
		for peer := range held {
			if peer != op.label {
				peers = append(peers, peer)
			}
		}

		slices.Sort(peers)

		for _, peer := range peers {
			var summary []float64
			payload := [2][][2]float64{held[peer], measured}
			for pointer := range op.leadlag.Next(data.NewValue(unsafe.Pointer(&payload)).Next(nil)) {
				summary = (*(*[2][]float64)(pointer))[0]
			}

			if err := op.leadlag.Error(); err != nil {
				op.Error(err)
				return
			}

			if len(summary) < 22 || summary[5] != 1 {
				nan := math.NaN()
				for i := 0; i < 20; i++ {
					if !yield(unsafe.Pointer(&nan)) { return }
				}
				continue
			}

			best_lag_correlation := summary[0]
			best_lag_covariance := summary[1]
			overlap_pair_count := summary[2]
			return_energy_reference := summary[3]
			return_energy_measured := summary[4]
			best_lag_index := summary[7]
			best_lag_seconds := summary[8]
			leads := summary[10]
			contemporaneous_correlation := summary[12]
			search_count := summary[13]
			lag_search_resolution_seconds := summary[14] * 1e-9
			lag_search_span := summary[15]
			pair_observation_count := summary[16]
			lag_search_scale := summary[17]
			absolute_correlation_gain := summary[18]
			lag_fraction := summary[19]
			lag_peak_prominence := summary[20]
			lag_peak_curvature := summary[21]

			fisher, known := op.fisher[peer]
			if !known {
				fisher = NewFisherEstimator()
				op.fisher[peer] = fisher
			}

			var history [10]float64
			for pointer := range fisher.Next(data.NewValue(unsafe.Pointer(&best_lag_correlation)).Next(nil)) {
				history = *(*[10]float64)(pointer)
			}

			if err := fisher.Error(); err != nil {
				op.Error(err)
				return
			}

			best_lag_correlation_baseline := history[2]
			best_lag_correlation_zscore := history[6]

			if !yield(unsafe.Pointer(&best_lag_correlation)) { return }
			if !yield(unsafe.Pointer(&best_lag_covariance)) { return }
			if !yield(unsafe.Pointer(&overlap_pair_count)) { return }
			if !yield(unsafe.Pointer(&return_energy_reference)) { return }
			if !yield(unsafe.Pointer(&return_energy_measured)) { return }
			if !yield(unsafe.Pointer(&best_lag_index)) { return }
			if !yield(unsafe.Pointer(&best_lag_seconds)) { return }
			if !yield(unsafe.Pointer(&leads)) { return }
			if !yield(unsafe.Pointer(&contemporaneous_correlation)) { return }
			if !yield(unsafe.Pointer(&search_count)) { return }
			if !yield(unsafe.Pointer(&lag_search_resolution_seconds)) { return }
			if !yield(unsafe.Pointer(&lag_search_span)) { return }
			if !yield(unsafe.Pointer(&pair_observation_count)) { return }
			if !yield(unsafe.Pointer(&lag_search_scale)) { return }
			if !yield(unsafe.Pointer(&absolute_correlation_gain)) { return }
			if !yield(unsafe.Pointer(&lag_fraction)) { return }
			if !yield(unsafe.Pointer(&lag_peak_prominence)) { return }
			if !yield(unsafe.Pointer(&lag_peak_curvature)) { return }
			if !yield(unsafe.Pointer(&best_lag_correlation_baseline)) { return }
			if !yield(unsafe.Pointer(&best_lag_correlation_zscore)) { return }
		}
	}
}
