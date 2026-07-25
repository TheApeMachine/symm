package logic

import (
	"math"

	"github.com/theapemachine/symm/logic/category"
	"github.com/theapemachine/symm/types"
)

/*
TrapEvidence is the relative trap vs opportunity mass for one symbol, derived
only from Thesis measurements already published by signals. Share is
trapMass / (trapMass + opportunityMass) when either mass is positive.
*/
type TrapEvidence struct {
	Share            float64
	TrapMass         float64
	OpportunityMass  float64
	Family           string
}

/*
Dominates reports that trap mass strictly exceeds opportunity mass.
*/
func (evidence TrapEvidence) Dominates() bool {
	return evidence.TrapMass > evidence.OpportunityMass && evidence.TrapMass > 0
}

/*
Tax returns the fraction of a non-negative return claim attributable to traps.
*/
func (evidence TrapEvidence) Tax(expectedReturn float64) float64 {
	if expectedReturn <= 0 || evidence.Share <= 0 {
		return 0
	}

	return evidence.Share * expectedReturn
}

/*
TrapShare scans published measurements for symbol and returns the relative trap
mass. Trap and opportunity families use each signal's normalized strength so the
ratio is scale-free across symbols.

ponytail: this full-book measurement scan is O(n) per symbol and becomes
O(symbols²) when hot-path consumers rescan independently; caching trap evidence
on the thesis or passing one computed result through forecast/decision is the
upgrade path.
*/
func TrapShare(thesis *types.Thesis, symbol string) TrapEvidence {
	if thesis == nil || symbol == "" {
		return TrapEvidence{}
	}

	var evidence TrapEvidence

	for _, measurement := range thesis.SnapshotMeasurements() {
		if measurement == nil ||
			measurement.Symbol != symbol ||
			len(measurement.Metrics) == 0 {
			continue
		}

		measurement.EachMetric(func(
			metric types.MetricType, _ types.MeasurementSide, sample types.MetricSample,
		) bool {
			mass, ok := trapSampleMass(sample)

			if !ok {
				return true
			}

			switch {
			case trapMetric(metric):
				if mass > evidence.TrapMass {
					evidence.TrapMass = mass
					evidence.Family = string(metric)
				}
			case opportunityMetric(metric):
				if mass > evidence.OpportunityMass {
					evidence.OpportunityMass = mass
				}
			}

			return true
		})
	}

	total := evidence.TrapMass + evidence.OpportunityMass

	if total <= 0 {
		return evidence
	}

	evidence.Share = evidence.TrapMass / total

	return evidence
}

/*
trapSampleMass returns abs(normalized) when present, otherwise abs(raw).
*/
func trapSampleMass(sample types.MetricSample) (float64, bool) {
	if sample.Normalized != nil {
		mass := math.Abs(*sample.Normalized)

		if mass <= 0 || math.IsNaN(mass) || math.IsInf(mass, 0) {
			return 0, false
		}

		return mass, true
	}

	mass := math.Abs(sample.Raw)

	if mass <= 0 || math.IsNaN(mass) || math.IsInf(mass, 0) {
		return 0, false
	}

	return mass, true
}

/*
trapMetric reports metrics whose normalized strength is trap evidence against a
long entry.
*/
func trapMetric(metric types.MetricType) bool {
	switch metric {
	case types.MetricAbsorption,
		types.MetricSpoofScore,
		types.MetricStarvation,
		types.MetricExhaustion:
		// RetreatingQuantity arms exit geometry (phantom adverse freeze), not
		// entry refusal — real pumps also lift resting asks. ThinScore is a
		// vacuum opportunity, not a trap. Book-only thin tapes mint no forecasts.
		return true
	default:
		return false
	}
}

/*
opportunityMetric reports metrics whose normalized strength corroborates a real
long opportunity rather than a phantom lift. Unbounded ratios (RVOL) are
excluded so mass comparison stays on shared signal family scales.
*/
func opportunityMetric(metric types.MetricType) bool {
	switch metric {
	case types.MetricIgnition,
		types.MetricTrend,
		types.MetricDrive,
		types.MetricSurgeScore,
		types.MetricCompression,
		types.MetricPrecursor:
		return true
	default:
		return false
	}
}

/*
CategoryTrap reads the resident category graph on Thesis and reports whether
trap categories / Contradicts edges dominate opportunity structure for symbol.
*/
func CategoryTrap(thesis *types.Thesis, symbol string) (share float64, dominates bool) {
	graph, ok := categoryGraph(thesis)

	if !ok {
		return 0, false
	}

	return category.Report(graph).TrapPressure(symbol)
}

/*
CategoryExhaustionLead reports whether Leads edges into exhaustion-family
categories dominate Leads into opportunity-family categories for symbol.
*/
func CategoryExhaustionLead(thesis *types.Thesis, symbol string) (share float64, dominates bool) {
	graph, ok := categoryGraph(thesis)

	if !ok {
		return 0, false
	}

	return category.Report(graph).ExhaustionLead(symbol)
}

/*
categoryGraph loads the resident category graph pointer from Thesis.Graphs.
*/
func categoryGraph(thesis *types.Thesis) (*category.Graph, bool) {
	if thesis == nil || thesis.Graphs == nil {
		return nil, false
	}

	value, ok := thesis.Graphs.Load("categories")

	if !ok {
		return nil, false
	}

	graph, ok := value.(*category.Graph)

	if !ok || graph == nil {
		return nil, false
	}

	return graph, true
}
