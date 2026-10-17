package audit

import (
	"context"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
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
RedundancyStage audits feature space orthogonality to identify collinear metric pairs.
When two metrics move in near-perfect lockstep (|r| >= 0.95), they are redundant clones
sharing representation subspace. This wastes memory/compute and creates numerical instability
(multicollinearity) in predictive models.
*/
type RedundancyStage struct {
	*runtime.System
	ch         chan *data.Value
	violations []*Violation
	pairs      map[string]*PairMoment
}

func NewRedundancyStage(ctx context.Context) *RedundancyStage {
	return &RedundancyStage{
		System:     runtime.NewSystem(ctx, "audit:redundancy"),
		ch:         make(chan *data.Value, 64),
		violations: make([]*Violation, 0),
		pairs:      make(map[string]*PairMoment),
	}
}

func (stage *RedundancyStage) Channel() chan *data.Value {
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

		if measurement.Origin == data.ExcursionOrigin || measurement.Origin == data.PolicyOrigin {
			continue
		}

		stage.auditMetricPairs(measurement)
	}

	stage.auditRedundancy()

	return nil
}

/*
auditMetricPairs extracts all metric values from the measurement
and updates online pairwise sufficient statistics.
*/
func (stage *RedundancyStage) auditMetricPairs(measurement *data.Value) {
	values := make(map[string]float64)

	maps.Copy(values, data.Metrics(measurement))

	metricKeys := make([]string, 0, len(values))
	for metricKey := range values {
		metricKeys = append(metricKeys, metricKey)
	}
	slices.Sort(metricKeys)

	for i := 0; i < len(metricKeys); i++ {
		for j := i + 1; j < len(metricKeys); j++ {
			pairKey := metricKeys[i] + " <-> " + metricKeys[j]

			moment, exists := stage.pairs[pairKey]
			if !exists {
				moment = &PairMoment{}
				stage.pairs[pairKey] = moment
			}

			moment.Update(values[metricKeys[i]], values[metricKeys[j]])
		}
	}
}

/*
auditRedundancy scans the bivariate correlation matrix across all pairs and flags
any pairs exceeding the collinearity threshold (|r| >= 0.95).
*/
func (stage *RedundancyStage) auditRedundancy() {
	if len(stage.pairs) == 0 {
		stage.violations = append(stage.violations, insufficient("redundancy", "no metric pairs observed"))
		return
	}

	pairKeys := make([]string, 0, len(stage.pairs))

	for pairKey := range stage.pairs {
		pairKeys = append(pairKeys, pairKey)
	}

	slices.Sort(pairKeys)

	for _, pairKey := range pairKeys {
		moment := stage.pairs[pairKey]

		correlation, ok := moment.Correlation()
		if !ok {
			continue
		}

		if algebraicPair(pairKey) {
			continue
		}

		if math.Abs(correlation) >= system.Cfg.Audit.CollinearCorrelation {
			stage.violations = append(stage.violations, NewViolation(
				pairKey,
				"redundancy.collinear_pair",
				fmt.Sprintf("Metric pair [%s] are collinear clones (|r| = %.4f >= %.2f across %.0f observations)",
					pairKey, correlation, system.Cfg.Audit.CollinearCorrelation, moment.Count),
				nil,
				nil,
			))
		}
	}
}

/*
algebraicPair is a pair whose lockstep is the definition of the two numbers,
not a second copy of one sensor. Best bid and best ask are the two sides of
one quote; the spread is the separate fact. The friction hurdle is the
relative spread plus a constant fee, so a correlation of 1 restates that
sum, which the truthfulness stage checks directly.
*/
func algebraicPair(pairKey string) bool {
	switch pairKey {
	case "best_price:ask <-> best_price:bid", "friction_hurdle <-> relative_spread":
		return true
	default:
		return false
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
	type PairSummary struct {
		Pair        string  `json:"pair"`
		Correlation float64 `json:"correlation"`
		Count       float64 `json:"count"`
		Collinear   bool    `json:"collinear"`
	}

	summaries := make([]PairSummary, 0, len(stage.pairs))
	pairKeys := make([]string, 0, len(stage.pairs))

	for pairKey := range stage.pairs {
		pairKeys = append(pairKeys, pairKey)
	}

	slices.Sort(pairKeys)

	for _, pairKey := range pairKeys {
		moment := stage.pairs[pairKey]
		corr, ok := moment.Correlation()
		if !ok {
			continue
		}

		summaries = append(summaries, PairSummary{
			Pair:        pairKey,
			Correlation: corr,
			Count:       moment.Count,
			Collinear:   !algebraicPair(pairKey) && math.Abs(corr) >= system.Cfg.Audit.CollinearCorrelation,
		})
	}

	collinear := 0

	for _, summary := range summaries {
		if summary.Collinear {
			collinear++
		}
	}

	return map[string]any{
		"passed":          len(stage.violations) == 0,
		"pairs_evaluated": len(summaries),
		"collinear_count": collinear,
		"pairs":           summaries,
	}
}
