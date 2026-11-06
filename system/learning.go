package system

import (
	"github.com/spf13/viper"
)

/*
Learning configures path-tree geometry and the resonance enter gate.
*/
type Learning struct {
	ThresholdStart float64
	ThresholdStep  float64
	ThresholdFloor float64
	ThresholdCeil  float64
	MaxPathLength  int
	MinPathLength  int
	// PolicyExitMinProfitPct is the fee-net PnL% floor for a tree exit leaf.
	PolicyExitMinProfitPct float64
	// ResistanceHoldHalfLifeSec scales win credit by hold duration in seconds
	// (shorter → more). Defaults to a few hours — typical lot lives.
	ResistanceHoldHalfLifeSec float64
	// ResistancePnLRefPct is the fee-net PnL% that earns full win credit.
	ResistancePnLRefPct      float64
	PaperEligibleResistance  float64
	MinSupportedHorizon      int
	MinTaskRelativePrecision float64
	MinSurprise              float64
	ResonanceBypass          bool
	MinEnterZ                float64
}

func NewLearning() *Learning {
	viper.SetDefault("learning.threshold_start", 0.5)
	viper.SetDefault("learning.threshold_step", 0.05)
	viper.SetDefault("learning.threshold_floor", 0.05)
	viper.SetDefault("learning.threshold_ceil", 0.95)
	viper.SetDefault("learning.max_path_length", 8)
	viper.SetDefault("learning.min_path_length", 2)
	viper.SetDefault("learning.policy_exit_min_profit_pct", 0.5)
	viper.SetDefault("learning.resistance_hold_half_life_sec", 14400)
	viper.SetDefault("learning.resistance_pnl_ref_pct", 1.0)
	viper.SetDefault("learning.paper_eligible_resistance", 0.2)
	viper.SetDefault("learning.min_supported_horizon", 2)
	viper.SetDefault("learning.min_task_relative_precision", 0.5)
	viper.SetDefault("learning.min_surprise", 0.5)
	viper.SetDefault("learning.resonance_bypass", false)
	viper.SetDefault("learning.min_enter_z", 0.35)

	return &Learning{
		ThresholdStart:            viper.GetFloat64("learning.threshold_start"),
		ThresholdStep:             viper.GetFloat64("learning.threshold_step"),
		ThresholdFloor:            viper.GetFloat64("learning.threshold_floor"),
		ThresholdCeil:             viper.GetFloat64("learning.threshold_ceil"),
		MaxPathLength:             viper.GetInt("learning.max_path_length"),
		MinPathLength:             viper.GetInt("learning.min_path_length"),
		PolicyExitMinProfitPct:    viper.GetFloat64("learning.policy_exit_min_profit_pct"),
		ResistanceHoldHalfLifeSec: viper.GetFloat64("learning.resistance_hold_half_life_sec"),
		ResistancePnLRefPct:       viper.GetFloat64("learning.resistance_pnl_ref_pct"),
		PaperEligibleResistance:   viper.GetFloat64("learning.paper_eligible_resistance"),
		MinSupportedHorizon:       viper.GetInt("learning.min_supported_horizon"),
		MinTaskRelativePrecision:  viper.GetFloat64("learning.min_task_relative_precision"),
		MinSurprise:               viper.GetFloat64("learning.min_surprise"),
		ResonanceBypass:           viper.GetBool("learning.resonance_bypass"),
		MinEnterZ:                 viper.GetFloat64("learning.min_enter_z"),
	}
}

/*
ApplyResonance patches live resonance-gate floors from a dashboard lab command.
*/
func (learning *Learning) ApplyResonance(patch map[string]any) {
	if learning == nil || len(patch) == 0 {
		return
	}

	setFloat := func(dst *float64, key string) {
		raw, ok := patch[key]
		if !ok || dst == nil {
			return
		}

		switch value := raw.(type) {
		case float64:
			if value == value {
				*dst = value
			}
		case int:
			*dst = float64(value)
		case int64:
			*dst = float64(value)
		}
	}

	setInt := func(dst *int, key string) {
		raw, ok := patch[key]
		if !ok || dst == nil {
			return
		}

		switch value := raw.(type) {
		case float64:
			if value == value {
				*dst = int(value)
			}
		case int:
			*dst = value
		case int64:
			*dst = int(value)
		}
	}

	if raw, ok := patch["bypass"]; ok {
		switch value := raw.(type) {
		case bool:
			learning.ResonanceBypass = value
		case float64:
			learning.ResonanceBypass = value != 0
		}
	}

	setFloat(&learning.MinTaskRelativePrecision, "min_task_relative_precision")
	setFloat(&learning.MinSurprise, "min_surprise")
	setFloat(&learning.MinEnterZ, "min_enter_z")
	setInt(&learning.MinSupportedHorizon, "min_supported_horizon")
}
