package strategy

import (
	"github.com/spf13/viper"
)

/*
Config holds live strategy thresholds. Loaded once at training construction;
the model reads these instead of hard-coded promotion constants.
*/
type Config struct {
	PolicyPrefix string

	MaxPathLength int
	// MinPathLength is retained so an existing config file still loads.
	// A closed enter-sided regime is a sample on its own; the market
	// does not require a path of this many tokens.
	MinPathLength int

	FreezeAfter    int
	ConfirmAfter   int
	DemotionLosses int

	PriorSamples  float64
	MinMeanReturn float64

	// A path starts at ThresholdStart. A losing exit raises it by
	// ThresholdStep and a winning exit lowers it, clamped to the floor
	// and the ceiling. Brightness has to clear the path's own threshold
	// before that path is entered. The ceiling prunes the path.
	ThresholdStart float64
	ThresholdStep  float64
	ThresholdFloor float64
	ThresholdCeil  float64

	MaxVenuePositions int

	RequireFrictionGate   bool
	FrictionMinBrightness float64
	MinTokenBrightness    float64
	MinGridMaturity       float64

	// Ignition and maturity stay on the tick as context. Entry uses the
	// path's own threshold, gauged by the regime's brightness.
	RequireIgnitionRegime bool
	IgnitionMinBrightness float64
}

func LoadConfig() *Config {
	viper.SetDefault("learning.policy_prefix", "policy/current/")
	viper.SetDefault("learning.max_path_length", 8)
	viper.SetDefault("learning.min_path_length", 3)
	viper.SetDefault("learning.freeze_after", 5)
	viper.SetDefault("learning.confirm_after", 3)
	viper.SetDefault("learning.demotion_losses", 3)
	viper.SetDefault("learning.prior_samples", 2.0)
	viper.SetDefault("learning.min_mean_return", 0.0)
	viper.SetDefault("learning.max_venue_positions", 5)
	viper.SetDefault("learning.threshold_start", 0.5)
	viper.SetDefault("learning.threshold_step", 0.05)
	viper.SetDefault("learning.threshold_floor", 0.05)
	viper.SetDefault("learning.threshold_ceil", 0.95)
	viper.SetDefault("learning.require_friction_gate", false)
	viper.SetDefault("learning.friction_min_brightness", 0.35)
	viper.SetDefault("learning.min_token_brightness", 0.25)
	viper.SetDefault("learning.min_grid_maturity", 0.35)
	viper.SetDefault("learning.require_ignition_regime", false)
	viper.SetDefault("learning.ignition_min_brightness", 0.35)

	return &Config{
		PolicyPrefix:          viper.GetString("learning.policy_prefix"),
		MaxPathLength:         viper.GetInt("learning.max_path_length"),
		MinPathLength:         viper.GetInt("learning.min_path_length"),
		FreezeAfter:           viper.GetInt("learning.freeze_after"),
		ConfirmAfter:          viper.GetInt("learning.confirm_after"),
		DemotionLosses:        viper.GetInt("learning.demotion_losses"),
		PriorSamples:          viper.GetFloat64("learning.prior_samples"),
		MinMeanReturn:         viper.GetFloat64("learning.min_mean_return"),
		ThresholdStart:        viper.GetFloat64("learning.threshold_start"),
		ThresholdStep:         viper.GetFloat64("learning.threshold_step"),
		ThresholdFloor:        viper.GetFloat64("learning.threshold_floor"),
		ThresholdCeil:         viper.GetFloat64("learning.threshold_ceil"),
		MaxVenuePositions:     viper.GetInt("learning.max_venue_positions"),
		RequireFrictionGate:   viper.GetBool("learning.require_friction_gate"),
		FrictionMinBrightness: viper.GetFloat64("learning.friction_min_brightness"),
		MinTokenBrightness:    viper.GetFloat64("learning.min_token_brightness"),
		MinGridMaturity:       viper.GetFloat64("learning.min_grid_maturity"),
		RequireIgnitionRegime: viper.GetBool("learning.require_ignition_regime"),
		IgnitionMinBrightness: viper.GetFloat64("learning.ignition_min_brightness"),
	}
}
