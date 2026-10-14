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
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
MetricVitality tracks running statistics for a single metric stream to determine
whether the metric is actively varying, stagnant, or dead (constant).
*/
type MetricVitality struct {
	Name      string
	Count     float64
	ZeroCount float64
	Min       float64
	Max       float64
	Moments   statistic.Moments
}

func (vitality *MetricVitality) Update(value float64) {
	if vitality.Count == 0 {
		vitality.Min = value
		vitality.Max = value
	}

	vitality.Count++
	vitality.Min = math.Min(vitality.Min, value)
	vitality.Max = math.Max(vitality.Max, value)

	if math.Abs(value) <= core.Resolution {
		vitality.ZeroCount++
	}

	vitality.Moments.Update(value)
}

func (vitality *MetricVitality) Variance() float64 {
	if vitality.Count < 2 {
		return 0
	}

	return vitality.Moments.M2 / (vitality.Count - 1)
}

func (vitality *MetricVitality) ZeroFraction() float64 {
	if vitality.Count == 0 {
		return 0
	}

	return vitality.ZeroCount / vitality.Count
}

/*
isStaticParameter returns true for venue configuration metrics (such as fee rate)
that are constant by design rather than dynamic market signals.
*/
func isStaticParameter(key string) bool {
	if strings.HasSuffix(key, "|fee_rate") || strings.HasSuffix(key, ":fee_rate") {
		return true
	}

	return false
}

/*
isRectifiedMetric returns true for one-sided rectified features (e.g. positive returns,
retreat fractions, or withdrawal rates) that are naturally sparse depending on tape direction.
*/
func isRectifiedMetric(key string) bool {
	if strings.Contains(key, "positive_") ||
		strings.Contains(key, "negative_") ||
		strings.Contains(key, "retreat_") ||
		strings.Contains(key, "withdrawal_") {
		return true
	}

	return false
}

/*
VitalityStage audits the health and variance of all incoming market metrics.
It detects "dead" metrics (zero variance, providing no informational value)
and "stagnant" cold-start metrics (>= 80% zero values).
*/
type VitalityStage struct {
	*runtime.System
	ch           chan *data.Measurement
	violations   []*Violation
	series       map[string]*MetricVitality
	flaggedDead  map[string]bool
	flaggedStagn map[string]bool
	totalTicks   int64
}

func NewVitalityStage(ctx context.Context) *VitalityStage {
	return &VitalityStage{
		System:       runtime.NewSystem(ctx, "audit:vitality"),
		ch:           make(chan *data.Measurement, 64),
		violations:   make([]*Violation, 0),
		series:       make(map[string]*MetricVitality),
		flaggedDead:  make(map[string]bool),
		flaggedStagn: make(map[string]bool),
	}
}

func (stage *VitalityStage) Channel() chan *data.Measurement {
	return stage.ch
}

func (stage *VitalityStage) Run() error {
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

		stage.recordMetrics(measurement)
	}

	stage.auditVitality()

	return nil
}

/*
recordMetrics updates the running Welford moments, min, max, and zero count
for each metric present in the measurement.
*/
func (stage *VitalityStage) recordMetrics(measurement *data.Measurement) {
	stage.totalTicks++

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

		key := measurement.Source + "|" + measurement.Label + "|" + entry.Metric.Label()
		stat, exists := stage.series[key]

		if !exists {
			stat = &MetricVitality{Name: key}
			stage.series[key] = stat
		}

		stat.Update(entry.Metric.Raw())
	}
}

/*
auditVitality scans accumulated metric populations for zero-variance (dead)
and stagnant (cold-start) series once minimum support is reached.
*/
func (stage *VitalityStage) auditVitality() {
	keys := make([]string, 0, len(stage.series))

	for key := range stage.series {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	for _, key := range keys {
		stat := stage.series[key]

		// Dead Metric Check: If a metric has accumulated enough samples (N >= 9)
		// but has ZERO variance (M2 == 0), it never changes. It carries zero entropy
		// and indicates a stalled calculation or dead feed.
		// Skip known static venue parameters like fee_rate.
		if stat.Count >= core.MinimumPrior && stat.Variance() == 0 && !isStaticParameter(key) && !stage.flaggedDead[key] {
			stage.flaggedDead[key] = true
			stage.violations = append(stage.violations, NewViolation(
				key,
				"vitality.dead_metric",
				fmt.Sprintf("Metric %s is dead/constant (variance = 0.0 across %.0f observations, range [%.3f, %.3f])",
					key, stat.Count, stat.Min, stat.Max),
				nil,
				nil,
			))
		}

		// Stagnant / Cold-Start Check: If >= 80% of observations are zero,
		// the metric is cold-starting or has insufficient event activity.
		// One-sided rectified metrics that have varied (Max > Min) are sparse by design, not stagnant.
		isRectified := isRectifiedMetric(key) && stat.Max > stat.Min

		if stat.Count >= core.MinimumPrior && stat.ZeroFraction() >= 0.80 && !isRectified && !stage.flaggedStagn[key] {
			stage.flaggedStagn[key] = true
			stage.violations = append(stage.violations, NewViolation(
				key,
				"vitality.stagnant_metric",
				fmt.Sprintf("Metric %s is stagnant/cold-start (zero fraction = %.1f%% across %.0f observations)",
					key, stat.ZeroFraction()*100, stat.Count),
				nil,
				nil,
			))
		}
	}
}

func (stage *VitalityStage) Violations() []*Violation {
	return stage.violations
}

func (stage *VitalityStage) Report() string {
	var build strings.Builder

	for _, violation := range stage.violations {
		build.WriteString(violation.String())
	}

	return build.String()
}

func (stage *VitalityStage) Data() any {
	stats := make(map[string]map[string]any, len(stage.series))
	deadList := make([]string, 0)
	stagnList := make([]string, 0)

	for name, metricVitality := range stage.series {
		variance := metricVitality.Variance()
		zeroFrac := metricVitality.ZeroFraction()

		stats[name] = map[string]any{
			"count":         metricVitality.Count,
			"min":           metricVitality.Min,
			"max":           metricVitality.Max,
			"variance":      variance,
			"zero_fraction": zeroFrac,
		}

		if stage.flaggedDead[name] {
			deadList = append(deadList, name)
		}

		if stage.flaggedStagn[name] {
			stagnList = append(stagnList, name)
		}
	}

	healthyCount := len(stage.series) - len(deadList) - len(stagnList)

	if healthyCount < 0 {
		healthyCount = 0
	}

	return map[string]any{
		"passed":           len(stage.violations) == 0,
		"total_metrics":    len(stage.series),
		"dead_count":       len(deadList),
		"stagnant_count":   len(stagnList),
		"healthy_count":    healthyCount,
		"dead_metrics":     deadList,
		"stagnant_metrics": stagnList,
		"metrics":          stats,
	}
}

