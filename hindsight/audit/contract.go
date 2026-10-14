package audit

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
ContractStage verifies hard mathematical domains declared by metric metadata,
Self-Normalized Movement (SNM) bound invariants, and measurement state invariants.
*/
type ContractStage struct {
	*runtime.System
	ch           chan *data.Measurement
	violations   []*Violation
	totalChecked int
}

func NewContractStage(ctx context.Context) *ContractStage {
	return &ContractStage{
		System: runtime.NewSystem(ctx, "audit:contract"),
		ch:     make(chan *data.Measurement, 64),
	}
}

func (stage *ContractStage) Channel() chan *data.Measurement {
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

		if measurement.Error() != nil {
			stage.violations = append(stage.violations, NewViolation(
				measurement.Label,
				"measurement.error",
				measurement.Error().Error(),
				measurement,
				nil,
			))
		}

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

			stage.totalChecked++

			switch entry.Metric.Unit() {
			// Asset prices cannot trade at zero or negative values.
			case data.UnitPrice:
				stage.validateBounds(
					entry.Metric.Raw(),
					entry.Metric,
					measurement,
					math.SmallestNonzeroFloat64,
					math.MaxFloat64,
					"Price must be strictly positive (> 0)",
				)

			// Physical counts, rates, variances, and volumes cannot physically be negative.
			case data.UnitQuantity,
				data.UnitVolume,
				data.UnitSpread,
				data.UnitDuration,
				data.UnitVariance,
				data.UnitStandardDeviation,
				data.UnitRate,
				data.UnitTradeRate,
				data.UnitVolumeRate,
				data.UnitCount,
				data.UnitEntropy,
				data.UnitSNR:
				stage.validateBounds(
					entry.Metric.Raw(),
					entry.Metric,
					measurement,
					0,
					math.MaxFloat64,
					"Physical quantity cannot be negative",
				)

			// Probabilities represent chances and must strictly stay between 0% and 100% [0, 1].
			case data.UnitProbability:
				stage.validateBounds(
					entry.Metric.Raw(),
					entry.Metric,
					measurement,
					0,
					1,
					"Probability must be bounded in [0, 1]",
				)

			// Correlations and imbalances measure directional co-movement from -100% (-1) to +100% (+1).
			case data.UnitCorrelation, data.UnitImbalance:
				stage.validateBounds(
					entry.Metric.Raw(),
					entry.Metric,
					measurement,
					-1,
					1,
					"Correlation must be bounded in [-1, 1]",
				)
			}

			// Self-Normalized Movement (SNM) normalizes signals onto a standard [-1, 1] range.
			stage.validateBounds(
				entry.Metric.SNM(),
				entry.Metric,
				measurement,
				-1,
				1,
				"Self-Normalized Movement must be bounded in [-1, 1]",
			)
		}
	}

	return nil
}

func (stage *ContractStage) Violations() []*Violation {
	return stage.violations
}

func (stage *ContractStage) validateBounds(
	value float64,
	metric *data.Metric,
	measurement *data.Measurement,
	min, max float64,
	description string,
) {
	if value < min || value > max {
		stage.violations = append(stage.violations, NewViolation(
			measurement.Label,
			metric.Label(),
			description,
			measurement,
			metric,
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

