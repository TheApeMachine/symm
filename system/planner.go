package system

import "github.com/spf13/viper"

/*
UninformativeDirectionConfidence is the probability at which a binary forecast
carries no evidence for either side.
*/
const UninformativeDirectionConfidence = 0.5

/* PlannerConfig contains only policy consumed by the live Planner. */
type PlannerConfig struct {
	MaxAllocationFraction     float64
	MinimumEntryProbability   float64
	CognitionSwitchConfidence float64
}

func NewPlannerConfig() *PlannerConfig {
	viper.SetDefault("trading.allocation.max_fraction", 0.1)
	viper.SetDefault("trading.admission.minimum_probability", 0.7)
	viper.SetDefault(
		"cognition.minimum_switch_confidence",
		UninformativeDirectionConfidence,
	)

	return &PlannerConfig{
		MaxAllocationFraction:     viper.GetFloat64("trading.allocation.max_fraction"),
		MinimumEntryProbability:   viper.GetFloat64("trading.admission.minimum_probability"),
		CognitionSwitchConfidence: viper.GetFloat64("cognition.minimum_switch_confidence"),
	}
}
