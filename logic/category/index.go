package category

import (
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
evidenceIndex is one SnapshotMeasurements pass keyed for category edge math.
*/
type evidenceIndex struct {
	symbols map[string]*symbolEvidence
}

/*
symbolEvidence holds per-metric mass and temporal envelopes for one symbol.
*/
type symbolEvidence struct {
	mass        map[types.MetricType]float64
	from        map[types.MetricType]time.Time
	through     map[types.MetricType]time.Time
	horizon     map[types.MetricType]time.Duration
	independent float64
	indepKeys   []string
}

/*
indexEvidence builds the per-symbol evidence index from one thesis snapshot.
*/
func indexEvidence(thesis *types.Thesis) *evidenceIndex {
	index := &evidenceIndex{symbols: map[string]*symbolEvidence{}}

	if thesis == nil {
		return index
	}

	for _, measurement := range thesis.SnapshotMeasurements() {
		if measurement == nil ||
			measurement.Validity.State != types.ValidityValid ||
			measurement.Symbol == "" ||
			len(measurement.Metrics) == 0 {
			continue
		}

		from, through := measurement.Interval()

		measurement.EachMetric(func(
			metric types.MetricType, _ types.MeasurementSide, sample types.MetricSample,
		) {
			mass, ok := sampleMass(sample)

			if !ok {
				return
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

			switch metric {
			case types.MetricDecoupled, types.MetricNoiseScore:
				bucket.independent += mass
				bucket.indepKeys = append(bucket.indepKeys, string(metric))
			}
		})
	}

	return index
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
independence returns live independence-bearing metric mass for symbol.
*/
func (index *evidenceIndex) independence(symbol string) (float64, []string) {
	if index == nil {
		return 0, nil
	}

	bucket := index.symbols[symbol]

	if bucket == nil {
		return 0, nil
	}

	return bucket.independent, append([]string(nil), bucket.indepKeys...)
}
