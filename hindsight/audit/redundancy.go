package audit

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
PairMoment tracks online bivariate sufficient statistics (means, variances, covariance)
between two metrics across streaming measurements using Welford's algorithm.
This lets us compute exact correlation on the fly without storing historical arrays.
*/
type PairMoment struct {
	Count float64
	MeanA float64
	MeanB float64
	M2A   float64
	M2B   float64
	CoMom float64
}

/*
Update applies Welford's bivariate recurrence to update the running averages,
spreads (sum of squared deviations), and co-moment with one new observation pair.
*/
func (pm *PairMoment) Update(valA, valB float64) {
	pm.Count++
	deltaA := valA - pm.MeanA
	pm.MeanA += deltaA / pm.Count
	deltaB := valB - pm.MeanB
	pm.MeanB += deltaB / pm.Count

	pm.M2A += deltaA * (valA - pm.MeanA)
	pm.M2B += deltaB * (valB - pm.MeanB)
	pm.CoMom += deltaA * (valB - pm.MeanB)
}

/*
Correlation returns the Pearson correlation coefficient between the two metrics:
r = CoMom / sqrt(M2A * M2B).
Returns false if sample size has not reached minimum rank prior (16 observations)
or if either metric is constant (variance is zero).
*/
func (pm *PairMoment) Correlation() (float64, bool) {
	if pm.Count < core.MinimumRankPrior || pm.M2A <= 0 || pm.M2B <= 0 {
		return 0, false
	}

	dispersion := math.Sqrt(pm.M2A * pm.M2B)

	if dispersion <= 0 {
		return 0, false
	}

	correlation := pm.CoMom / dispersion

	return correlation, true
}

/*
isTrivialPair returns true for metric pairs that are collinear by definition
or track identical underlying asset prices (such as ask and bid levels).
*/
func isTrivialPair(firstMetric, secondMetric string) bool {
	if (strings.Contains(firstMetric, "best_ask_price") && strings.Contains(secondMetric, "best_bid_price")) ||
		(strings.Contains(firstMetric, "best_bid_price") && strings.Contains(secondMetric, "best_ask_price")) {
		return true
	}

	if (strings.Contains(firstMetric, "friction_hurdle") && strings.Contains(secondMetric, "relative_spread")) ||
		(strings.Contains(firstMetric, "relative_spread") && strings.Contains(secondMetric, "friction_hurdle")) {
		return true
	}

	return false
}

/*
RedundancyStage audits feature space orthogonality to identify collinear metric pairs.
When two metrics move in near-perfect lockstep (|r| >= 0.95), they are redundant clones
sharing representation subspace. This wastes memory/compute and creates numerical instability
(multicollinearity) in predictive models.
*/
type RedundancyStage struct {
	*runtime.System
	ch         chan *data.Measurement
	violations []*Violation
	pairs      map[string]*PairMoment
}

func NewRedundancyStage(ctx context.Context) *RedundancyStage {
	return &RedundancyStage{
		System:     runtime.NewSystem(ctx, "audit:redundancy"),
		ch:         make(chan *data.Measurement, 64),
		violations: make([]*Violation, 0),
		pairs:      make(map[string]*PairMoment),
	}
}

func (stage *RedundancyStage) Channel() chan *data.Measurement {
	return stage.ch
}

func (stage *RedundancyStage) Run() error {
	errnie.Info(fmt.Sprintf("%s: Running audit...", stage.Name()))

	for measurement := range stage.ch {
		if measurement == nil {
			stage.violations = append(stage.violations, NewViolation(
				"unknown",
				"measurement",
				"Nil measurement encountered in tape",
				nil,
				nil,
			))

			continue
		}

		if measurement.Source == "runtime:join" {
			continue
		}

		if measurement.Error() != nil {
			stage.violations = append(stage.violations, NewViolation(
				measurement.Label,
				"measurement.error",
				measurement.Error().Error(),
				measurement,
				nil,
			))
		}

		stage.auditMetricPairs(measurement)
	}

	stage.auditRedundancy()

	return nil
}

/*
auditMetricPairs extracts all metric values from the measurement and updates
online pairwise sufficient statistics.
*/
func (stage *RedundancyStage) auditMetricPairs(measurement *data.Measurement) {
	values := make(map[string]float64)

	for entry := range measurement.Read() {
		if entry == nil || entry.Metric == nil {
			stage.violations = append(stage.violations, NewViolation(
				measurement.Label,
				"entry.metric",
				"Nil metric entry encountered in measurement",
				measurement,
				nil,
			))

			continue
		}

		values[entry.Metric.Label()] = entry.Metric.Raw()
	}

	metricKeys := make([]string, 0, len(values))

	for metricKey := range values {
		metricKeys = append(metricKeys, metricKey)
	}

	slices.Sort(metricKeys)

	// Compare every unique pair of metrics (A, B) where A < B.
	for outerIdx := 0; outerIdx < len(metricKeys); outerIdx++ {
		for innerIdx := outerIdx + 1; innerIdx < len(metricKeys); innerIdx++ {
			keyA := metricKeys[outerIdx]
			keyB := metricKeys[innerIdx]
			pairKey := measurement.Label + "|" + keyA + "<->" + keyB

			moment, exists := stage.pairs[pairKey]

			if !exists {
				moment = &PairMoment{}
				stage.pairs[pairKey] = moment
			}

			moment.Update(values[keyA], values[keyB])
		}
	}
}

/*
auditRedundancy evaluates mature correlation across the entire stream at the end of the run.
This avoids false positives from early-sample latching when N is small.
*/
func (stage *RedundancyStage) auditRedundancy() {
	pairKeys := make([]string, 0, len(stage.pairs))

	for pairKey := range stage.pairs {
		pairKeys = append(pairKeys, pairKey)
	}

	slices.Sort(pairKeys)

	for _, pairKey := range pairKeys {
		moment := stage.pairs[pairKey]
		correlation, scorable := moment.Correlation()

		if !scorable || math.Abs(correlation) < 0.95 {
			continue
		}

		parts := strings.Split(pairKey, "|")
		label := ""
		pair := pairKey

		if len(parts) >= 2 {
			label = parts[0]
			pair = parts[1]
		}

		metricParts := strings.Split(pair, "<->")

		if len(metricParts) == 2 && isTrivialPair(metricParts[0], metricParts[1]) {
			continue
		}

		stage.violations = append(stage.violations, NewViolation(
			label,
			"redundancy.collinear_pair",
			fmt.Sprintf("Metrics in %s are collinear clones (|r| = %.4f >= 0.95)",
				pair, correlation),
			nil,
			nil,
		))
	}
}

func (stage *RedundancyStage) Violations() []*Violation {
	return stage.violations
}

func (stage *RedundancyStage) Report() string {
	var build strings.Builder

	for _, violation := range stage.violations {
		build.WriteString(violation.String())
	}

	return build.String()
}

func (stage *RedundancyStage) Data() any {
	redundantPairs := make([]map[string]any, 0)
	pairKeys := make([]string, 0, len(stage.pairs))

	for pairKey := range stage.pairs {
		pairKeys = append(pairKeys, pairKey)
	}

	slices.Sort(pairKeys)

	for _, pairKey := range pairKeys {
		moment := stage.pairs[pairKey]
		correlation, scorable := moment.Correlation()

		if !scorable || math.Abs(correlation) < 0.95 {
			continue
		}

		parts := strings.Split(pairKey, "|")
		pair := pairKey

		if len(parts) >= 2 {
			pair = parts[1]
		}

		metricParts := strings.Split(pair, "<->")

		if len(metricParts) == 2 && isTrivialPair(metricParts[0], metricParts[1]) {
			continue
		}

		redundantPairs = append(redundantPairs, map[string]any{
			"pair":        pairKey,
			"correlation": correlation,
			"count":       moment.Count,
		})
	}

	return map[string]any{
		"passed":          len(redundantPairs) == 0,
		"total_pairs":     len(stage.pairs),
		"redundant_count": len(redundantPairs),
		"redundant_pairs": redundantPairs,
	}
}

