package system

import "github.com/spf13/viper"

/* UninformativeDirectionConfidence is exact probability indifference. */
const UninformativeDirectionConfidence = 0.5

/* PlannerConfig contains only policy consumed by the live Planner. */
type PlannerConfig struct {
	MaxAllocationFraction     float64
	CognitionSwitchConfidence float64
}

func NewPlannerConfig() *PlannerConfig {
	viper.SetDefault("trading.allocation.max_fraction", 0.1)
	viper.SetDefault(
		"cognition.minimum_switch_confidence",
		UninformativeDirectionConfidence,
	)

	return &PlannerConfig{
		MaxAllocationFraction:     viper.GetFloat64("trading.allocation.max_fraction"),
		CognitionSwitchConfidence: viper.GetFloat64("cognition.minimum_switch_confidence"),
	}
}
