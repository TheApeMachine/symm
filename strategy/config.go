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
	MinPathLength int

	FreezeAfter  int
	ConfirmAfter int
	DemotionLosses int

	PriorSamples   float64
	MinMeanReturn  float64

	MaxVenuePositions int

	RequireFrictionGate bool
	FrictionMinBrightness float64
	MinTokenBrightness    float64
	MinGridMaturity       float64
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
	viper.SetDefault("learning.require_friction_gate", false)
	viper.SetDefault("learning.friction_min_brightness", 0.35)
	viper.SetDefault("learning.min_token_brightness", 0.25)
	viper.SetDefault("learning.min_grid_maturity", 0.35)

	return &Config{
		PolicyPrefix:          viper.GetString("learning.policy_prefix"),
		MaxPathLength:         viper.GetInt("learning.max_path_length"),
		MinPathLength:         viper.GetInt("learning.min_path_length"),
		FreezeAfter:           viper.GetInt("learning.freeze_after"),
		ConfirmAfter:          viper.GetInt("learning.confirm_after"),
		DemotionLosses:        viper.GetInt("learning.demotion_losses"),
		PriorSamples:          viper.GetFloat64("learning.prior_samples"),
		MinMeanReturn:         viper.GetFloat64("learning.min_mean_return"),
		MaxVenuePositions:     viper.GetInt("learning.max_venue_positions"),
		RequireFrictionGate:   viper.GetBool("learning.require_friction_gate"),
		FrictionMinBrightness: viper.GetFloat64("learning.friction_min_brightness"),
		MinTokenBrightness:    viper.GetFloat64("learning.min_token_brightness"),
		MinGridMaturity:       viper.GetFloat64("learning.min_grid_maturity"),
	}
}
