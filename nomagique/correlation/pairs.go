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
Pairs measures the focal symbol's retained price path against every peer path
held in the shared path store (see Member), without peers ever living on the
Measurement. The focal path is the measured path Y and each peer is the
reference path X. Per pair it composes Dependence around the supplied
asynchronous estimator and a per-peer FisherEstimator for the correlation's
causal history; across all defined pairs it composes Cohort.

Each arrival is the *data.Adapter. Pair facts are published into the adapter
under "<fact>@<peer>"; cohort facts are published unsuffixed. A pair whose
estimator is undefined publishes nothing. The adapter is yielded unchanged.
*/
type Pairs struct {
	*core.PrimitiveError
	label      string
	paths      core.Primitive
	dependence core.Primitive
	cohort     core.Primitive
	fisher     map[string]core.Primitive
	request    map[string][][2]float64
	rows       [][3]float64
	output     data.Map[float64]
}

func NewPairs(label string, paths core.Primitive, estimator core.Primitive) core.Primitive {
	return &Pairs{
		PrimitiveError: core.NewPrimitiveError(),
		label:          label,
		paths:          paths,
		dependence:     NewDependence(estimator),
		cohort:         NewCohort(),
		fisher:         make(map[string]core.Primitive),
		request:        make(map[string][][2]float64),
		output:         data.NewOutputMap(),
	}
}

func (op *Pairs) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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
			op.rows = op.rows[:0]
			focalRate := math.NaN()

			for _, peer := range peers {
				var dependence [13]float64

				for pointer := range op.dependence.Next(data.NewValue([2][][2]float64{held[peer], measured})) {
					dependence = *(*[13]float64)(pointer)
				}

				if err := op.dependence.Error(); err != nil {
					op.Error(err)
					return
				}

				if dependence[5] != 1 {
					continue
				}

				suffix := "@" + peer
				correlation := dependence[0]

				op.output.Values["signed_correlation"+suffix] = correlation
				op.output.Values["absolute_correlation"+suffix] = math.Abs(correlation)
				op.output.Values["covariance"+suffix] = dependence[1]
				op.output.Values["overlap_pair_count"+suffix] = dependence[2]
				op.output.Values["return_energy:reference"+suffix] = dependence[3]
				op.output.Values["return_energy:measured"+suffix] = dependence[4]
				op.output.Values["return_count:reference"+suffix] = dependence[6]
				op.output.Values["return_count:measured"+suffix] = dependence[7]
				op.output.Values["return_energy_rate:reference"+suffix] = dependence[8]
				op.output.Values["return_energy_rate:measured"+suffix] = dependence[9]
				op.output.Values["shared_time"+suffix] = dependence[11]
				op.output.Values["overlap_density"+suffix] = dependence[12]

				if dependence[8] > 0 && dependence[9] > 0 {
					op.output.Values["relative_return_energy"+suffix] = dependence[9] / dependence[8]
				}

				fisher, known := op.fisher[peer]

				if !known {
					fisher = NewFisherEstimator()
					op.fisher[peer] = fisher
				}

				var history [10]float64

				for pointer := range fisher.Next(data.NewValue(correlation)) {
					history = *(*[10]float64)(pointer)
				}

				if err := fisher.Error(); err != nil {
					op.Error(err)
					return
				}

				op.output.Values["correlation_baseline"+suffix] = history[2]
				op.output.Values["correlation_divergence"+suffix] = history[3]
				op.output.Values["correlation_zscore"+suffix] = history[6]

				focalRate = dependence[9]
				op.rows = append(op.rows, [3]float64{correlation, dependence[2], dependence[8]})
			}

			if len(op.rows) > 0 {
				var cohort [11]float64

				for pointer := range op.cohort.Next(data.NewValue(op.rows...)) {
					cohort = *(*[11]float64)(pointer)
				}

				if err := op.cohort.Error(); err != nil {
					op.Error(err)
					return
				}

				op.output.Values["cohort_peer_count"] = cohort[1]

				if cohort[9] == 1 {
					op.output.Values["cohort_effective_peer_count"] = cohort[4]
					op.output.Values["cohort_signed_correlation"] = cohort[5]
					op.output.Values["cohort_absolute_correlation"] = cohort[6]
					op.output.Values["peer_return_energy_rate"] = cohort[7]
					op.output.Values["focal_return_energy_rate"] = focalRate

					if cohort[7] > 0 && focalRate > 0 {
						op.output.Values["relative_cohort_return_energy"] = focalRate / cohort[7]
					}
				}

				if cohort[10] == 1 {
					op.output.Values["cohort_correlation_dispersion"] = cohort[8]
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

/*
Relations retains measured pair facts. Pending Measurement-store migration it
pass-through yields arrivals unchanged.
*/
type Relations struct {
	*core.PrimitiveError
}

func NewRelations() core.Primitive {
	return &Relations{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Relations) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if !yield(arriving) {
				return
			}
		}
	}
}
