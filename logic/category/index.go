package category

import (
	"math"
	"sync"
	"time"

	"github.com/theapemachine/symm/types"
)

/*
contradictIndex maps from→to category onto the metrics whose affinity Supports
from and Opposes to. Built once from CategoryAffinity so edge derivation does
not rescan the affinity table per pair.
*/
var contradictIndex = buildContradictIndex()

/*
buildContradictIndex indexes affinity contradictions at package init.
*/
func buildContradictIndex() map[types.CategoryType]map[types.CategoryType][]types.MetricType {
	index := map[types.CategoryType]map[types.CategoryType][]types.MetricType{}

	for metric, affinity := range types.CategoryAffinity {
		for _, from := range affinity.Supports {
			for _, to := range affinity.Opposes {
				targets := index[from]

				if targets == nil {
					targets = map[types.CategoryType][]types.MetricType{}
					index[from] = targets
				}

				targets[to] = append(targets[to], metric)
			}
		}
	}

	return index
}

/*
sampleMass returns abs(normalized) when present, otherwise abs(raw), for graph
edge evidence derived straight from the Thesis measurement surface.
*/
func sampleMass(sample types.MetricSample) (float64, bool) {
	if sample.Normalized != nil {
		mass := math.Abs(*sample.Normalized)

		return mass, mass > 0 && !math.IsNaN(mass) && !math.IsInf(mass, 0)
	}

	mass := math.Abs(sample.Raw)

	return mass, mass > 0 && !math.IsNaN(mass) && !math.IsInf(mass, 0)
}

/*
evidenceIndex is one SnapshotMeasurements pass keyed for category edge math.
*/
type evidenceIndex struct {
	symbols map[string]*symbolEvidence
}

/*
newEvidenceIndex allocates the resident evidence index that Graph reuses across
cuts. Evidence is derived from the moving Thesis, so only map contents reset;
headers stay alive to avoid rebuilding the same per-symbol containers each tick.
*/
func newEvidenceIndex() *evidenceIndex {
	return &evidenceIndex{symbols: map[string]*symbolEvidence{}}
}

/*
symbolEvidence holds per-metric mass and temporal envelopes for one symbol.
*/
type symbolEvidence struct {
	mass    map[types.MetricType]float64
	from    map[types.MetricType]time.Time
	through map[types.MetricType]time.Time
	horizon map[types.MetricType]time.Duration
}

/*
clear drops metric evidence while retaining the maps for the next Thesis pass.
*/
func (evidence *symbolEvidence) clear() {
	for metric := range evidence.mass {
		delete(evidence.mass, metric)
	}

	for metric := range evidence.from {
		delete(evidence.from, metric)
	}

	for metric := range evidence.through {
		delete(evidence.through, metric)
	}

	for metric := range evidence.horizon {
		delete(evidence.horizon, metric)
	}
}

/*
UpdateFrom refreshes the resident evidence index from the moving Thesis. The
Thesis owns current measurements, and the index only stores derived mass and
temporal envelopes needed by graph edge classification.
*/
func (index *evidenceIndex) UpdateFrom(thesis *types.Thesis) {
	if thesis == nil {
		return
	}

	index.UpdateMeasurements(thesis.Measurements)
}

/*
UpdateMeasurements refreshes per-symbol metric evidence without reallocating the
top-level index or per-symbol buckets.
*/
func (index *evidenceIndex) UpdateMeasurements(measurements *sync.Map) {
	if index.symbols == nil {
		index.symbols = map[string]*symbolEvidence{}
	}

	for _, bucket := range index.symbols {
		bucket.clear()
	}

	measurements.Range(func(_, value any) bool {
		measurement, ok := value.(*types.Measurement)

		if !ok || measurement == nil {
			return true
		}

		index.update(measurement)

		return true
	})
}

func (index *evidenceIndex) update(measurement *types.Measurement) {
	if measurement == nil ||
		measurement.Validity.State != types.ValidityValid ||
		measurement.Symbol == "" ||
		len(measurement.Metrics) == 0 {
		return
	}

	from, through := measurement.Interval()

	measurement.EachMetric(func(
		metric types.MetricType, _ types.MeasurementSide, sample types.MetricSample,
	) bool {
		mass, ok := sampleMass(sample)

		if !ok {
			return true
		}

		bucket := index.symbols[measurement.Symbol]

		if bucket == nil {
			bucket = &symbolEvidence{
				mass:    map[types.MetricType]float64{},
				from:    map[types.MetricType]time.Time{},
				through: map[types.MetricType]time.Time{},
				horizon: map[types.MetricType]time.Duration{},
			}
			index.symbols[measurement.Symbol] = bucket
		}

		if mass > bucket.mass[metric] {
			bucket.mass[metric] = mass
		}

		if previous, ok := bucket.from[metric]; !ok || from.Before(previous) {
			bucket.from[metric] = from
		}

		if previous, ok := bucket.through[metric]; !ok || through.After(previous) {
			bucket.through[metric] = through
		}

		if measurement.Horizon > bucket.horizon[metric] {
			bucket.horizon[metric] = measurement.Horizon
		}

		return true
	})
}

/*
metricMass returns indexed abs-normalized mass for one metric on symbol.
*/
func (index *evidenceIndex) metricMass(symbol string, metric types.MetricType) float64 {
	if index == nil {
		return 0
	}

	bucket := index.symbols[symbol]

	if bucket == nil {
		return 0
	}

	return bucket.mass[metric]
}

/*
clockFor builds an evidence clock from indexed supporting metrics.
*/
func (index *evidenceIndex) clockFor(symbol string, metrics []string) evidenceClock {
	if index == nil || len(metrics) == 0 {
		return evidenceClock{}
	}

	bucket := index.symbols[symbol]

	if bucket == nil {
		return evidenceClock{}
	}

	var clock evidenceClock

	for _, name := range metrics {
		metric := types.MetricType(name)
		mass := bucket.mass[metric]

		if mass <= 0 {
			continue
		}

		from := bucket.from[metric]
		through := bucket.through[metric]

		if !clock.ok {
			clock.from = from
			clock.through = through
			clock.ok = true
		}

		if from.Before(clock.from) {
			clock.from = from
		}

		if through.After(clock.through) {
			clock.through = through
		}

		if bucket.horizon[metric] > clock.horizon {
			clock.horizon = bucket.horizon[metric]
		}

		clock.mass += mass
	}

	return clock
}

/*
independence returns live independence-bearing metric mass for symbol when at
least one category in the pair is supported by a decoupled or noise metric.
*/
func (index *evidenceIndex) independence(
	symbol string,
	first, second types.CategoryType,
) float64 {
	if index == nil {
		return 0
	}

	bucket := index.symbols[symbol]

	if bucket == nil {
		return 0
	}

	mass := 0.0

	for _, metric := range independentMetrics {
		value := bucket.mass[metric]

		if value <= 0 {
			continue
		}

		if !independenceEligible(metric, first) && !independenceEligible(metric, second) {
			continue
		}

		mass += value
	}

	return mass
}

/*
appendIndependence appends live independence metric names into caller-owned
scratch after independence mass proves the edge exists. That keeps the evidence
copy at the retained relation destination and removes per-pair slice literals.
*/
func (index *evidenceIndex) appendIndependence(
	evidence []string,
	symbol string,
	first, second types.CategoryType,
) []string {
	bucket := index.symbols[symbol]

	for _, metric := range independentMetrics {
		if bucket.mass[metric] <= 0 {
			continue
		}

		if !independenceEligible(metric, first) && !independenceEligible(metric, second) {
			continue
		}

		evidence = append(evidence, string(metric))
	}

	return evidence
}

/*
independenceEligible reports whether metric affinity supports categoryType.
*/
func independenceEligible(metric types.MetricType, categoryType types.CategoryType) bool {
	affinity, ok := types.AffinityFor(metric)

	if !ok {
		return false
	}

	for _, supported := range affinity.Supports {
		if supported == categoryType {
			return true
		}
	}

	return false
}
