package system

import (
	"time"

	"github.com/spf13/viper"
)

/*
Learning configures resource use, persistence cadence, and opt-in rehearsal
augmentation, not market beliefs.

RehearsalDropout additionally teaches every rehearsed context once with one
of its region frames missing and once with one frame swapped for another
region of the same fragment tape (deliberate noise, as dropout does). It is
off unless enabled: the perturbed contexts never occurred in the market.
*/
type Learning struct {
	PolicyPrefix            string
	EnterSuffix             string
	ExitSuffix              string
	MinimumPathConfidence   int
	ThresholdStart          float64
	ThresholdStep           float64
	ExploreTemporalPenalty  float64
	ThresholdFloor          float64
	ThresholdCeil           float64
	VirtualPromote          int
	VocabSize               int
	DefaultWindowSize       int
	ShannonCapacity         float64
	MaxPathLength           int
	MinPathLength           int
	FreezeAfter             int
	ConfirmAfter            int
	DemotionLosses          int
	PriorSamples            float64
	MinMeanReturn           float64
	MaxVenuePositions       int
	RequireFrictionGate     bool
	FrictionMinBrightness   float64
	MinTokenBrightness      float64
	MinGridMaturity         float64
	RequireIgnitionRegime   bool
	IgnitionMinBrightness   float64
	FuzzyMatchEnabled       bool
	FuzzyMatchThreshold     float64
	SalienceRatio           float64
	ExcursionTimeout        time.Duration
	MaxExcursionBars        int
	TimeoutLossPenalty      float64
	EmergencyExitBrightness float64
	MinSupportedHorizon     int
	CliffHorizonDrop        int
	CliffEnergyMult         float64
	PositionTracePath       string
	HistoryEvery            time.Duration
	HistoryCap              int
	HistoryBatch            int
	// GateHurdleRate is the learning rate of the Gate's regressing hurdles:
	// each settled round trip moves a hurdle eta*(target - hurdle).
	GateHurdleRate float64
	// GateIgnitionFloor / GateIgnitionCeil bound the minimum ignition required
	// to earn a Paper entry (self-normalized brightness, 0..1).
	GateIgnitionFloor float64
	GateIgnitionCeil  float64
	// GateRiskFloor / GateRiskCeil bound the maximum risk-to-net-spark tolerated
	// for a Paper entry (brightness units).
	GateRiskFloor float64
	GateRiskCeil  float64
	// GateRejectionFloor / GateRejectionCeil bound the minimum rejection force
	// required to take a profitable exit (brightness units).
	GateRejectionFloor float64
	GateRejectionCeil  float64
	// PathPruneResistance is the resistance threshold above which a path is
	// pruned from both the tree and object storage: the trade that pushed it
	// past is also a realized loss, so the path has earned its retirement.
	PathPruneResistance float64
	// MaxPathObjects bounds the number of path objects retained in the tree.
	MaxPathObjects int
}

func NewLearning() *Learning {
	viper.SetDefault("learning.policy_prefix", "model/current/")
	viper.SetDefault("learning.enter_suffix", "/enter.bin")
	viper.SetDefault("learning.exit_suffix", "/exit.bin")
	viper.SetDefault("learning.minimum_path_confidence", 2)
	viper.SetDefault("learning.threshold_start", 0.5)
	viper.SetDefault("learning.threshold_step", 0.05)
	viper.SetDefault("learning.explore_temporal_penalty", 0.001)
	viper.SetDefault("learning.threshold_floor", 0.05)
	viper.SetDefault("learning.threshold_ceil", 0.95)
	viper.SetDefault("learning.virtual_promote", 3)
	viper.SetDefault("learning.vocab_size", 15)
	viper.SetDefault("learning.default_window_size", 15)
	viper.SetDefault("learning.shannon_capacity", 0.45)
	viper.SetDefault("learning.max_path_length", 8)
	viper.SetDefault("learning.min_path_length", 2)
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
	viper.SetDefault("learning.require_ignition_regime", false)
	viper.SetDefault("learning.ignition_min_brightness", 0.35)
	viper.SetDefault("learning.fuzzy_match_enabled", true)
	viper.SetDefault("learning.fuzzy_match_threshold", 0.75)
	viper.SetDefault("learning.salience_ratio", 0.40)
	viper.SetDefault("learning.excursion_timeout", 45*time.Minute)
	viper.SetDefault("learning.max_excursion_bars", 8)
	viper.SetDefault("learning.timeout_loss_penalty", 0.02)
	viper.SetDefault("learning.emergency_exit_brightness", 0.75)
	viper.SetDefault("learning.min_supported_horizon", 2)
	viper.SetDefault("learning.cliff_horizon_drop", 2)
	viper.SetDefault("learning.cliff_energy_mult", 1.4)
	viper.SetDefault("learning.position_trace_path", "")
	viper.SetDefault("learning.history_every", 5*time.Second)
	viper.SetDefault("learning.history_cap", 20000)
	viper.SetDefault("learning.history_batch", 200)
	viper.SetDefault("learning.gate_hurdle_rate", 0.10)
	viper.SetDefault("learning.gate_ignition_floor", 0.15)
	viper.SetDefault("learning.gate_ignition_ceil", 1.00)
	viper.SetDefault("learning.gate_risk_floor", 0.50)
	viper.SetDefault("learning.gate_risk_ceil", 5.00)
	viper.SetDefault("learning.gate_rejection_floor", 0.30)
	viper.SetDefault("learning.gate_rejection_ceil", 4.00)
	viper.SetDefault("learning.path_prune_resistance", 0.80)
	viper.SetDefault("learning.max_path_objects", 8192)

	return &Learning{
		PolicyPrefix:            viper.GetString("learning.policy_prefix"),
		EnterSuffix:             viper.GetString("learning.enter_suffix"),
		ExitSuffix:              viper.GetString("learning.exit_suffix"),
		MinimumPathConfidence:   viper.GetInt("learning.minimum_path_confidence"),
		ThresholdStart:          viper.GetFloat64("learning.threshold_start"),
		ThresholdStep:           viper.GetFloat64("learning.threshold_step"),
		ExploreTemporalPenalty:  viper.GetFloat64("learning.explore_temporal_penalty"),
		ThresholdFloor:          viper.GetFloat64("learning.threshold_floor"),
		ThresholdCeil:           viper.GetFloat64("learning.threshold_ceil"),
		VirtualPromote:          viper.GetInt("learning.virtual_promote"),
		VocabSize:               viper.GetInt("learning.vocab_size"),
		DefaultWindowSize:       viper.GetInt("learning.default_window_size"),
		ShannonCapacity:         viper.GetFloat64("learning.shannon_capacity"),
		MaxPathLength:           viper.GetInt("learning.max_path_length"),
		MinPathLength:           viper.GetInt("learning.min_path_length"),
		FreezeAfter:             viper.GetInt("learning.freeze_after"),
		ConfirmAfter:            viper.GetInt("learning.confirm_after"),
		DemotionLosses:          viper.GetInt("learning.demotion_losses"),
		PriorSamples:            viper.GetFloat64("learning.prior_samples"),
		MinMeanReturn:           viper.GetFloat64("learning.min_mean_return"),
		MaxVenuePositions:       viper.GetInt("learning.max_venue_positions"),
		RequireFrictionGate:     viper.GetBool("learning.require_friction_gate"),
		FrictionMinBrightness:   viper.GetFloat64("learning.friction_min_brightness"),
		MinTokenBrightness:      viper.GetFloat64("learning.min_token_brightness"),
		MinGridMaturity:         viper.GetFloat64("learning.min_grid_maturity"),
		RequireIgnitionRegime:   viper.GetBool("learning.require_ignition_regime"),
		IgnitionMinBrightness:   viper.GetFloat64("learning.ignition_min_brightness"),
		FuzzyMatchEnabled:       viper.GetBool("learning.fuzzy_match_enabled"),
		FuzzyMatchThreshold:     viper.GetFloat64("learning.fuzzy_match_threshold"),
		SalienceRatio:           viper.GetFloat64("learning.salience_ratio"),
		ExcursionTimeout:        viper.GetDuration("learning.excursion_timeout"),
		MaxExcursionBars:        viper.GetInt("learning.max_excursion_bars"),
		TimeoutLossPenalty:      viper.GetFloat64("learning.timeout_loss_penalty"),
		EmergencyExitBrightness: viper.GetFloat64("learning.emergency_exit_brightness"),
		MinSupportedHorizon:     viper.GetInt("learning.min_supported_horizon"),
		CliffHorizonDrop:        viper.GetInt("learning.cliff_horizon_drop"),
		CliffEnergyMult:         viper.GetFloat64("learning.cliff_energy_mult"),
		PositionTracePath:       viper.GetString("learning.position_trace_path"),
		HistoryEvery:            viper.GetDuration("learning.history_every"),
		HistoryCap:              viper.GetInt("learning.history_cap"),
		HistoryBatch:            viper.GetInt("learning.history_batch"),
		GateHurdleRate:          viper.GetFloat64("learning.gate_hurdle_rate"),
		GateIgnitionFloor:       viper.GetFloat64("learning.gate_ignition_floor"),
		GateIgnitionCeil:        viper.GetFloat64("learning.gate_ignition_ceil"),
		GateRiskFloor:           viper.GetFloat64("learning.gate_risk_floor"),
		GateRiskCeil:            viper.GetFloat64("learning.gate_risk_ceil"),
		GateRejectionFloor:      viper.GetFloat64("learning.gate_rejection_floor"),
		GateRejectionCeil:       viper.GetFloat64("learning.gate_rejection_ceil"),
		PathPruneResistance:     viper.GetFloat64("learning.path_prune_resistance"),
		MaxPathObjects:          viper.GetInt("learning.max_path_objects"),
	}
}
