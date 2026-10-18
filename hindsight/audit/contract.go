package audit

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
ContractStage verifies hard mathematical domains declared by metric metadata,
Self-Normalized Movement (SNM) bound invariants, and measurement state invariants.
*/
type ContractStage struct {
	*runtime.System
	ch           chan *data.Value
	violations   []*Violation
	totalChecked int
}

func NewContractStage(ctx context.Context) *ContractStage {
	return &ContractStage{
		System: runtime.NewSystem(ctx, "audit:contract"),
		ch:     make(chan *data.Value, 64),
	}
}

func (stage *ContractStage) Channel() chan *data.Value {
	return stage.ch
}

func (stage *ContractStage) Run() error {
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

		metrics := floatMetrics(measurement)
		for label, val := range metrics {
			stage.totalChecked++

			min, max, description, applies := metricDomain(label)
			if !applies {
				continue
			}

			stage.validateBounds(val, label, measurement, min, max, description)
		}
	}

	if stage.totalChecked == 0 {
		stage.violations = append(stage.violations, insufficient("contract", "no metrics observed"))
	}

	return nil
}

func (stage *ContractStage) Violations() []*Violation {
	return stage.violations
}

/*
metricDomain is the range a metric is allowed to take. Returns are matched
before prices: a name like midpoint_log_return contains "midpoint" but it is
a signed change, and zero or a down-move is a real result, not a broken price.
One-sided return names (positive_ / negative_) store a size, so they may be
zero and must not be negative. A bare price or midpoint is a level and must
be above zero.
*/
func metricDomain(label string) (min, max float64, description string, applies bool) {
	lower := strings.ToLower(label)

	if strings.Contains(lower, "return") {
		if strings.HasPrefix(lower, "positive_") || strings.HasPrefix(lower, "negative_") {
			return 0, math.MaxFloat64, "One-sided return cannot be negative", true
		}

		return math.Inf(-1), math.Inf(1), "Return must be a real number", true
	}

	if strings.Contains(lower, "corr") || strings.Contains(lower, "imbalance") || strings.Contains(lower, "signed_net_fraction") || lower == "breadth" {
		return -1, 1, "Correlation must be bounded in [-1, 1]", true
	}

	if strings.Contains(lower, "percentile") || strings.Contains(lower, "prob") || strings.Contains(lower, "confidence") || strings.Contains(lower, "excitation_fraction") {
		return 0, 1, "Probability must be bounded in [0, 1]", true
	}

	if strings.Contains(lower, "price") || strings.Contains(lower, "midpoint") {
		return math.SmallestNonzeroFloat64, math.MaxFloat64, "Price must be strictly positive (> 0)", true
	}

	if strings.Contains(lower, "qty") || strings.Contains(lower, "volume") || strings.Contains(lower, "count") || strings.Contains(lower, "spread") || strings.Contains(lower, "duration") || strings.Contains(lower, "rate") {
		return 0, math.MaxFloat64, "Physical quantity cannot be negative", true
	}

	return 0, 0, "", false
}

func (stage *ContractStage) validateBounds(
	value float64,
	label string,
	measurement *data.Value,
	min, max float64,
	description string,
) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < min || value > max {
		scope := "unknown"
		if measurement != nil {
			scope = measurement.Scope
		}
		stage.violations = append(stage.violations, NewViolation(
			scope,
			label,
			description,
			measurement,
			label,
		))
	}
}

func (stage *ContractStage) Report() string {
	var build strings.Builder

	for _, violation := range stage.violations {
		build.WriteString(violation.String())
	}

	return build.String()
}

func (stage *ContractStage) Data() any {
	breaches := make([]map[string]any, 0, len(stage.violations))

	for _, violation := range stage.violations {
		breaches = append(breaches, map[string]any{
			"symbol":      violation.Symbol,
			"name":        violation.Name,
			"description": violation.Description,
		})
	}

	return map[string]any{
		"passed":        len(stage.violations) == 0,
		"total_checked": stage.totalChecked,
		"breach_count":  len(stage.violations),
		"breaches":      breaches,
	}
}
