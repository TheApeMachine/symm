package correlation

import (
	"iter"
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
	output  data.Map[float64]
}

func NewLeads(label string, paths core.Primitive, estimator core.Primitive) core.Primitive {
	return &Leads{
		PrimitiveError: core.NewPrimitiveError(),
		label:          label,
		paths:          paths,
		leadlag:        NewLeadLag(estimator),
		fisher:         make(map[string]core.Primitive),
		request:        make(map[string][][2]float64),
		output:         data.NewOutputMap(),
	}
}

func (op *Leads) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			var held map[string][][2]float64

			for pointer := range op.paths.Next(data.NewValue(op.request)) {
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
			clear(op.output.Values)

			for _, peer := range peers {
				var summary []float64

				for pointer := range op.leadlag.Next(data.NewValue([2][][2]float64{held[peer], measured})) {
					summary = (*(*[2][]float64)(pointer))[0]
				}

				if err := op.leadlag.Error(); err != nil {
					op.Error(err)
					return
				}

				if len(summary) < 22 || summary[5] != 1 {
					continue
				}

				suffix := "@" + peer

				op.output.Values["best_lag_correlation"+suffix] = summary[0]
				op.output.Values["best_lag_covariance"+suffix] = summary[1]
				op.output.Values["overlap_pair_count"+suffix] = summary[2]
				op.output.Values["return_energy:reference"+suffix] = summary[3]
				op.output.Values["return_energy:measured"+suffix] = summary[4]
				op.output.Values["best_lag_index"+suffix] = summary[7]
				op.output.Values["best_lag_seconds"+suffix] = summary[8]
				op.output.Values["leads"+suffix] = summary[10]
				op.output.Values["contemporaneous_correlation"+suffix] = summary[12]
				op.output.Values["search_count"+suffix] = summary[13]
				op.output.Values["lag_search_resolution_seconds"+suffix] = summary[14] * 1e-9
				op.output.Values["lag_search_span"+suffix] = summary[15]
				op.output.Values["pair_observation_count"+suffix] = summary[16]
				op.output.Values["lag_search_scale"+suffix] = summary[17]
				op.output.Values["absolute_correlation_gain"+suffix] = summary[18]
				op.output.Values["lag_fraction"+suffix] = summary[19]

				if summary[11] == 1 {
					op.output.Values["lag_peak_prominence"+suffix] = summary[20]
					op.output.Values["lag_peak_curvature"+suffix] = summary[21]
				}

				fisher, known := op.fisher[peer]

				if !known {
					fisher = NewFisherEstimator()
					op.fisher[peer] = fisher
				}

				var history [10]float64

				for pointer := range fisher.Next(data.NewValue(summary[0])) {
					history = *(*[10]float64)(pointer)
				}

				if err := fisher.Error(); err != nil {
					op.Error(err)
					return
				}

				if history[1] == 1 && history[9] == 1 {
					op.output.Values["best_lag_correlation_baseline"+suffix] = history[2]
				}

				if history[1] == 1 && history[8] == 1 {
					op.output.Values["best_lag_correlation_zscore"+suffix] = history[6]
				}
			}

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
