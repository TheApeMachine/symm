package system

import (
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

/* UninformativeDirectionConfidence is exact probability indifference. */
const UninformativeDirectionConfidence = 0.5

/* PlannerConfig contains only policy consumed by the live Planner. */
type PlannerConfig struct {
	MaxAllocationFraction     float64
	MaxLossFraction           float64
	AggregateMaxLossFraction  float64
	CognitionSwitchConfidence float64
}

func NewPlannerConfig() *PlannerConfig {
	viper.SetDefault("trading.allocation.max_fraction", 0.20)
	viper.SetDefault("trading.allocation.max_loss_fraction", 0.02)
	viper.SetDefault("trading.allocation.aggregate_max_loss_fraction", 0.06)
	viper.SetDefault(
		"cognition.minimum_switch_confidence",
		UninformativeDirectionConfidence,
	)

	maxLoss := viper.GetFloat64("trading.allocation.max_loss_fraction")

	if maxLoss <= 0 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"[planner] max_loss_fraction must be positive",
			nil,
		))
	}

	aggLoss := viper.GetFloat64("trading.allocation.aggregate_max_loss_fraction")

	if aggLoss <= 0 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"[planner] aggregate_max_loss_fraction must be positive",
			nil,
		))
	}

	return &PlannerConfig{
		MaxAllocationFraction:     viper.GetFloat64("trading.allocation.max_fraction"),
		MaxLossFraction:           maxLoss,
		AggregateMaxLossFraction:  aggLoss,
		CognitionSwitchConfidence: viper.GetFloat64("cognition.minimum_switch_confidence"),
	}
}
