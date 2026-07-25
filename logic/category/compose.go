package category

import (
	"math"
	"sort"
	"time"

	"github.com/theapemachine/symm/types"
)

/*
Compose turns valid Thesis measurements for one symbol into Category rows via
CategoryAffinity. Categories are hypotheses over evidence, not signal winners:
each metric contributes support or oppose mass, missing required support is
recorded, and strength is the relative support share after that accounting.
*/
func Compose(thesis *types.Thesis, symbol string) []types.Category {
	if thesis == nil || symbol == "" {
		return nil
	}

	rows := make([]*types.Measurement, 0, 16)

	for _, measurement := range thesis.SnapshotMeasurements() {
		if measurement == nil || measurement.Symbol != symbol {
			continue
		}

		rows = append(rows, measurement)
	}

	return composeRows(symbol, thesis.At, rows)
}

/*
ComposeAll builds composed categories for every symbol in one measurement
snapshot pass so a fat thesis is not rescanned per symbol.
*/
func ComposeAll(thesis *types.Thesis) []types.Category {
	if thesis == nil {
		return nil
	}

	grouped := map[string][]*types.Measurement{}

	for _, measurement := range thesis.SnapshotMeasurements() {
		if measurement == nil || measurement.Symbol == "" {
			continue
		}

		if !hasAffinityMeasurement(measurement) {
			continue
		}

		grouped[measurement.Symbol] = append(grouped[measurement.Symbol], measurement)
	}

	categories := make([]types.Category, 0, len(grouped)*4)

	for symbol, rows := range grouped {
		categories = append(categories, composeRows(symbol, thesis.At, rows)...)
	}

	return categories
}

/*
composeRows turns one symbol's measurement rows into active Category states.
*/
func composeRows(
	symbol string, at time.Time, rows []*types.Measurement,
) []types.Category {
	if symbol == "" || len(rows) == 0 {
		return nil
	}

	accumulators := map[types.CategoryType]*accumulator{}
	present := map[types.MetricType]struct{}{}

	for _, measurement := range rows {
		if !usable(measurement, symbol) {
			continue
		}

		if measurement.At.After(at) {
			at = measurement.At
		}

		measurement.EachMetric(func(
			metric types.MetricType, _ types.MeasurementSide, sample types.MetricSample,
		) {
			affinity, ok := types.AffinityFor(metric)

			if !ok {
				return
			}

			mass, ok := sampleMass(sample)

			if !ok {
				return
			}

			present[metric] = struct{}{}

			for _, categoryType := range affinity.Supports {
				ensure(accumulators, categoryType).support(
					string(metric), mass, measurement,
				)
			}

			for _, categoryType := range affinity.Opposes {
				ensure(accumulators, categoryType).oppose(
					string(metric), mass, measurement,
				)
			}
		})
	}

	if len(accumulators) == 0 {
		return nil
	}

	categories := make([]types.Category, 0, len(accumulators))

	for categoryType, accumulator := range accumulators {
		if accumulator == nil {
			continue
		}

		missing := missingSupport(requiredMetricsTable[categoryType], present)
		support := accumulator.supportMass
		oppose := accumulator.opposeMass
		missingPenalty := support * float64(len(missing)) /
			float64(len(missing)+len(accumulator.supporting)+1)
		denominator := support + oppose + missingPenalty
		strength := 0.0

		if denominator > 0 {
			strength = support / denominator
		}

		if strength <= 0 || support <= 0 {
			continue
		}

		maturity := accumulator.maturity
		uncertainty := accumulator.uncertainty
		freshness := maturity

		if !at.IsZero() && !accumulator.latest.IsZero() {
			span := at.Sub(accumulator.latest)

			if span > 0 && accumulator.horizon > 0 {
				freshness = 1 / (1 + float64(span)/float64(accumulator.horizon))
			}
		}

		confidence := strength * (1 / (1 + uncertainty))
		categories = append(categories, types.Category{
			Symbol:      symbol,
			Type:        categoryType,
			Confidence:  confidence,
			Strength:    strength,
			Maturity:    maturity,
			Uncertainty: uncertainty,
			Freshness:   freshness,
			Supporting:  sortedCopy(accumulator.supporting),
			Opposing:    sortedCopy(accumulator.opposing),
			Missing:     missing,
		})
	}

	sort.Slice(categories, func(left, right int) bool {
		if categories[left].Strength == categories[right].Strength {
			return categories[left].Type < categories[right].Type
		}

		return categories[left].Strength > categories[right].Strength
	})

	return categories
}

/*
ensure returns the accumulator for categoryType, allocating on first touch.
*/
func ensure(
	accumulators map[types.CategoryType]*accumulator,
	categoryType types.CategoryType,
) *accumulator {
	bucket := accumulators[categoryType]

	if bucket == nil {
		bucket = &accumulator{}
		accumulators[categoryType] = bucket
	}

	return bucket
}

/*
accumulator gathers support/oppose mass and provenance for one category.
*/
type accumulator struct {
	supportMass float64
	opposeMass  float64
	supporting  []string
	opposing    []string
	maturity    float64
	uncertainty float64
	latest      time.Time
	horizon     time.Duration
	seenSupport map[string]struct{}
	seenOppose  map[string]struct{}
}

/*
support records supporting evidence with diminishing returns when multiple
metrics name the same category, so redundant observables do not inflate mass
linearly.
*/
func (accumulator *accumulator) support(
	metric string, mass float64, measurement *types.Measurement,
) {
	if accumulator.seenSupport == nil {
		accumulator.seenSupport = map[string]struct{}{}
	}

	if _, exists := accumulator.seenSupport[metric]; exists {
		return
	}

	accumulator.seenSupport[metric] = struct{}{}
	redundancy := 1 / (1 + float64(len(accumulator.supporting)))
	accumulator.supportMass += mass * redundancy
	accumulator.supporting = append(accumulator.supporting, metric)
	accumulator.observe(measurement)
}

/*
oppose records opposing evidence with the same redundancy adjustment.
*/
func (accumulator *accumulator) oppose(
	metric string, mass float64, measurement *types.Measurement,
) {
	if accumulator.seenOppose == nil {
		accumulator.seenOppose = map[string]struct{}{}
	}

	if _, exists := accumulator.seenOppose[metric]; exists {
		return
	}

	accumulator.seenOppose[metric] = struct{}{}
	redundancy := 1 / (1 + float64(len(accumulator.opposing)))
	accumulator.opposeMass += mass * redundancy
	accumulator.opposing = append(accumulator.opposing, metric)
	accumulator.observe(measurement)
}

/*
observe keeps maturity, uncertainty, and temporal anchors from evidence rows.
*/
func (accumulator *accumulator) observe(measurement *types.Measurement) {
	if measurement.Maturity > accumulator.maturity {
		accumulator.maturity = measurement.Maturity
	}

	if measurement.Uncertainty != nil {
		width := math.Abs(measurement.Uncertainty.Upper-measurement.Uncertainty.Lower) / 2

		if width > accumulator.uncertainty {
			accumulator.uncertainty = width
		}
	}

	if measurement.At.After(accumulator.latest) {
		accumulator.latest = measurement.At
	}

	if measurement.Horizon > accumulator.horizon {
		accumulator.horizon = measurement.Horizon
	}
}

/*
usable reports a valid measurement row for the target symbol.
*/
func usable(measurement *types.Measurement, symbol string) bool {
	return measurement != nil &&
		measurement.Symbol == symbol &&
		len(measurement.Metrics) > 0 &&
		measurement.Validity.State == types.ValidityValid
}

/*
hasAffinityMeasurement reports whether any Metrics entry maps to CategoryAffinity.
*/
func hasAffinityMeasurement(measurement *types.Measurement) bool {
	if measurement == nil || len(measurement.Metrics) == 0 {
		return false
	}

	found := false

	measurement.EachMetric(func(metric types.MetricType, _ types.MeasurementSide, _ types.MetricSample) {
		if _, ok := types.AffinityFor(metric); ok {
			found = true
		}
	})

	return found
}

/*
sampleMass returns abs(normalized) when present, otherwise abs(raw).
*/
func sampleMass(sample types.MetricSample) (float64, bool) {
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
requiredMetricsTable indexes every metric that lists a category as Supports.
*/
var requiredMetricsTable = buildRequiredMetrics()

/*
buildRequiredMetrics indexes support requirements once from CategoryAffinity.
*/
func buildRequiredMetrics() map[types.CategoryType][]types.MetricType {
	required := map[types.CategoryType][]types.MetricType{}

	for metric, affinity := range types.CategoryAffinity {
		for _, categoryType := range affinity.Supports {
			required[categoryType] = append(required[categoryType], metric)
		}
	}

	return required
}

/*
missingSupport lists required support metrics absent from the present set.
*/
func missingSupport(
	required []types.MetricType, present map[types.MetricType]struct{},
) []string {
	if len(required) == 0 {
		return nil
	}

	missing := make([]string, 0, len(required))

	for _, metric := range required {
		if _, ok := present[metric]; ok {
			continue
		}

		missing = append(missing, string(metric))
	}

	sort.Strings(missing)

	return missing
}

/*
sortedCopy returns a sorted copy of keys for stable thesis/UI rows.
*/
func sortedCopy(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	copied := append([]string(nil), values...)
	sort.Strings(copied)

	return copied
}

/*
Top returns the strongest composed category for symbol, or empty.
*/
func Top(categories []types.Category, symbol string) types.Category {
	for _, category := range categories {
		if category.Symbol == symbol && category.Type != types.CategoryTypeNone {
			return category
		}
	}

	return types.Category{}
}
