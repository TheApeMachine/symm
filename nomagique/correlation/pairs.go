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
	}
}

func (op *Pairs) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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
		op.rows = op.rows[:0]
		focalRate := 0.0

		for _, peer := range peers {
			var dependence [13]float64

			payload := [2][][2]float64{held[peer], measured}
			for pointer := range op.dependence.Next(data.NewValue(unsafe.Pointer(&payload)).Next(nil)) {
				dependence = *(*[13]float64)(pointer)
			}

			if err := op.dependence.Error(); err != nil {
				op.Error(err)
				return
			}

			if dependence[5] != 1 {
				zero := 0.0
				for i := 0; i < 16; i++ {
					if !yield(unsafe.Pointer(&zero)) { return }
				}
				continue
			}

			correlation := dependence[0]
			abs_correlation := math.Abs(correlation)
			covariance := dependence[1]
			overlap_pair_count := dependence[2]
			return_energy_reference := dependence[3]
			return_energy_measured := dependence[4]
			return_count_reference := dependence[6]
			return_count_measured := dependence[7]
			return_energy_rate_reference := dependence[8]
			return_energy_rate_measured := dependence[9]
			shared_time := dependence[11]
			overlap_density := dependence[12]
			
			relative_return_energy := 0.0
			if dependence[8] > 0 && dependence[9] > 0 {
				relative_return_energy = dependence[9] / dependence[8]
			}

			fisher, known := op.fisher[peer]
			if !known {
				fisher = NewFisherEstimator()
				op.fisher[peer] = fisher
			}

			var history [10]float64
			for pointer := range fisher.Next(data.NewValue(unsafe.Pointer(&correlation)).Next(nil)) {
				history = *(*[10]float64)(pointer)
			}

			if err := fisher.Error(); err != nil {
				op.Error(err)
				return
			}

			correlation_baseline := history[2]
			correlation_divergence := history[3]
			correlation_zscore := history[6]

			focalRate = dependence[9]
			op.rows = append(op.rows, [3]float64{correlation, dependence[2], dependence[8]})

			if !yield(unsafe.Pointer(&correlation)) { return }
			if !yield(unsafe.Pointer(&abs_correlation)) { return }
			if !yield(unsafe.Pointer(&covariance)) { return }
			if !yield(unsafe.Pointer(&overlap_pair_count)) { return }
			if !yield(unsafe.Pointer(&return_energy_reference)) { return }
			if !yield(unsafe.Pointer(&return_energy_measured)) { return }
			if !yield(unsafe.Pointer(&return_count_reference)) { return }
			if !yield(unsafe.Pointer(&return_count_measured)) { return }
			if !yield(unsafe.Pointer(&return_energy_rate_reference)) { return }
			if !yield(unsafe.Pointer(&return_energy_rate_measured)) { return }
			if !yield(unsafe.Pointer(&shared_time)) { return }
			if !yield(unsafe.Pointer(&overlap_density)) { return }
			if !yield(unsafe.Pointer(&relative_return_energy)) { return }
			if !yield(unsafe.Pointer(&correlation_baseline)) { return }
			if !yield(unsafe.Pointer(&correlation_divergence)) { return }
			if !yield(unsafe.Pointer(&correlation_zscore)) { return }
		}

		zero := 0.0
		if len(op.rows) == 0 {
			for i := 0; i < 8; i++ {
				if !yield(unsafe.Pointer(&zero)) { return }
			}
			return
		}

		var cohort [11]float64
		for pointer := range op.cohort.Next(data.NewValue(unsafe.Pointer(&op.rows)).Next(nil)) {
			cohort = *(*[11]float64)(pointer)
		}

		if err := op.cohort.Error(); err != nil {
			op.Error(err)
			return
		}

		cohort_peer_count := cohort[1]
		cohort_effective_peer_count := 0.0
		cohort_signed_correlation := 0.0
		cohort_absolute_correlation := 0.0
		peer_return_energy_rate := 0.0
		focal_return_energy_rate := 0.0
		relative_cohort_return_energy := 0.0
		cohort_correlation_dispersion := 0.0

		if cohort[9] == 1 {
			cohort_effective_peer_count = cohort[4]
			cohort_signed_correlation = cohort[5]
			cohort_absolute_correlation = cohort[6]
			peer_return_energy_rate = cohort[7]
			focal_return_energy_rate = focalRate

			if cohort[7] > 0 && focalRate > 0 {
				relative_cohort_return_energy = focalRate / cohort[7]
			}
		}

		if cohort[10] == 1 {
			cohort_correlation_dispersion = cohort[8]
		}

		if !yield(unsafe.Pointer(&cohort_peer_count)) { return }
		if !yield(unsafe.Pointer(&cohort_effective_peer_count)) { return }
		if !yield(unsafe.Pointer(&cohort_signed_correlation)) { return }
		if !yield(unsafe.Pointer(&cohort_absolute_correlation)) { return }
		if !yield(unsafe.Pointer(&peer_return_energy_rate)) { return }
		if !yield(unsafe.Pointer(&focal_return_energy_rate)) { return }
		if !yield(unsafe.Pointer(&relative_cohort_return_energy)) { return }
		if !yield(unsafe.Pointer(&cohort_correlation_dispersion)) { return }
	}
}