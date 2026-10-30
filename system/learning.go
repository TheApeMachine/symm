package system

import (
	"time"

	"github.com/spf13/viper"
)

/*
Learning configures resource use, persistence cadence, and tokenizer geometry.
Market hurdles (gate, path fuzzy match, stops) are learned at runtime and
persist under model/parameters/gate.bin — not toggled from here.
*/
type Learning struct {
	PolicyPrefix           string
	EnterSuffix            string
	ExitSuffix             string
	MinimumPathConfidence  int
	ThresholdStart         float64
	ThresholdStep          float64
	ExploreTemporalPenalty float64
	ThresholdFloor         float64
	ThresholdCeil          float64
	VocabSize              int
	DefaultWindowSize      int
	ShannonCapacity        float64
	MaxPathLength          int
	MinPathLength          int
	FreezeAfter            int
	ConfirmAfter           int
	DemotionLosses         int
	PriorSamples           float64
	MinMeanReturn          float64
	MaxVenuePositions      int
	ExcursionTimeout       time.Duration
	MaxExcursionBars       int
	PolicyShadowBars       int
	MinSupportedHorizon    int
	CliffHorizonDrop       int
	CliffEnergyMult        float64
	PositionTracePath      string
	HistoryEvery           time.Duration
	HistoryCap             int
	HistoryBatch           int
	GateHurdleRate         float64
	GateIgnitionFloor      float64
	GateIgnitionCeil       float64
	GateRiskFloor          float64
	GateRiskCeil           float64
	GateRejectionFloor     float64
	GateRejectionCeil      float64
	GateOppositionFloor    float64
	GateOppositionCeil     float64
	GateRegimeIgnitionFloor float64
	GateRegimeIgnitionCeil  float64
	GateRegimeFrictionFloor float64
	GateRegimeFrictionCeil  float64
	GateMaturityFloor      float64
	GateMaturityCeil       float64
	MaxPathObjects         int
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
	viper.SetDefault("learning.excursion_timeout", 45*time.Minute)
	viper.SetDefault("learning.max_excursion_bars", 8)
	viper.SetDefault("learning.policy_shadow_bars", 32)
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
	viper.SetDefault("learning.gate_opposition_floor", 0.05)
	viper.SetDefault("learning.gate_opposition_ceil", 0.95)
	viper.SetDefault("learning.gate_regime_ignition_floor", 0.05)
	viper.SetDefault("learning.gate_regime_ignition_ceil", 0.95)
	viper.SetDefault("learning.gate_regime_friction_floor", 0.05)
	viper.SetDefault("learning.gate_regime_friction_ceil", 0.95)
	viper.SetDefault("learning.gate_maturity_floor", 0.05)
	viper.SetDefault("learning.gate_maturity_ceil", 0.95)
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
		ExcursionTimeout:        viper.GetDuration("learning.excursion_timeout"),
		MaxExcursionBars:        viper.GetInt("learning.max_excursion_bars"),
		PolicyShadowBars:        viper.GetInt("learning.policy_shadow_bars"),
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
		GateOppositionFloor:     viper.GetFloat64("learning.gate_opposition_floor"),
		GateOppositionCeil:      viper.GetFloat64("learning.gate_opposition_ceil"),
		GateRegimeIgnitionFloor: viper.GetFloat64("learning.gate_regime_ignition_floor"),
		GateRegimeIgnitionCeil:  viper.GetFloat64("learning.gate_regime_ignition_ceil"),
		GateRegimeFrictionFloor: viper.GetFloat64("learning.gate_regime_friction_floor"),
		GateRegimeFrictionCeil:  viper.GetFloat64("learning.gate_regime_friction_ceil"),
		GateMaturityFloor:       viper.GetFloat64("learning.gate_maturity_floor"),
		GateMaturityCeil:        viper.GetFloat64("learning.gate_maturity_ceil"),
		MaxPathObjects:          viper.GetInt("learning.max_path_objects"),
	}
}
