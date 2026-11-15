package system

import "github.com/spf13/viper"

/*
Selection configures cross-sectional board ranking, capacity limits, dual peak
caps, ignition thresholds, and profit-taking knobs.
*/
type Selection struct {
	MaxOpenPositions int
	MaxEntersPerTick int
	MaxAccumPeakFrac float64
	MaxIgnitionPeakFrac float64
	IgnitionPrecursorMin    float64
	IgnitionCompressionMin  float64
	IgnitionIgnitionMin     float64
	WeightPeakScore         float64
	WeightPrecursor         float64
	WeightTapeUptake        float64
	WeightSpread            float64
	WeightPeakFrac          float64
	WeightRotation          float64
	HeartbeatBatch          int
	StoplossArmPnlPct         float64
	StoplossRateDecayMin      float64
	StoplossPumpConfirmMin    float64
	StoplossConfirmMarks      int
	StoplossNearPeakFrac      float64
	// StoplossGivebackPct is fee-net PnL percentage points given back from
	// peak before an armed trail exit may confirm (grind and rolled climbers).
	StoplossGivebackPct float64
	// StoplossGrindVelocityMax is peak climb velocity (%/sec) below which an
	// armed position is treated as grind instead of crest-stall.
	StoplossGrindVelocityMax float64
	// StoplossGrindMinSamples is ascent sample depth before grind may classify.
	StoplossGrindMinSamples int
	RotationCloseBoost      float64
}

func NewSelection() *Selection {
	viper.SetDefault("selection.max_open_positions", 5)
	viper.SetDefault("selection.max_enters_per_tick", 1)
	viper.SetDefault("selection.max_accum_peak_frac", 0.35)
	viper.SetDefault("selection.max_ignition_peak_frac", 0.85)
	viper.SetDefault("selection.ignition_precursor_min", 0.55)
	viper.SetDefault("selection.ignition_compression_min", 0.55)
	viper.SetDefault("selection.ignition_ignition_min", 0.45)
	viper.SetDefault("selection.weight_peak_score", 1.25)
	viper.SetDefault("selection.weight_precursor", 1.0)
	viper.SetDefault("selection.weight_tape_uptake", 0.75)
	viper.SetDefault("selection.weight_spread", 1.0)
	viper.SetDefault("selection.weight_peak_frac", 0.5)
	viper.SetDefault("selection.weight_rotation", 0.35)
	viper.SetDefault("selection.heartbeat_batch", 48)
	viper.SetDefault("selection.stoploss_arm_pnl_pct", 1.0)
	viper.SetDefault("selection.stoploss_rate_decay_min", 0.7)
	viper.SetDefault("selection.stoploss_pump_confirm_min", 0.35)
	viper.SetDefault("selection.stoploss_confirm_marks", 3)
	viper.SetDefault("selection.stoploss_near_peak_frac", 0.98)
	viper.SetDefault("selection.stoploss_giveback_pct", 0.5)
	viper.SetDefault("selection.stoploss_grind_velocity_max", 0.05)
	viper.SetDefault("selection.stoploss_grind_min_samples", 8)
	viper.SetDefault("selection.rotation_close_boost", 0.15)

	return &Selection{
		MaxOpenPositions:         viper.GetInt("selection.max_open_positions"),
		MaxEntersPerTick:         viper.GetInt("selection.max_enters_per_tick"),
		MaxAccumPeakFrac:         viper.GetFloat64("selection.max_accum_peak_frac"),
		MaxIgnitionPeakFrac:      viper.GetFloat64("selection.max_ignition_peak_frac"),
		IgnitionPrecursorMin:     viper.GetFloat64("selection.ignition_precursor_min"),
		IgnitionCompressionMin:   viper.GetFloat64("selection.ignition_compression_min"),
		IgnitionIgnitionMin:      viper.GetFloat64("selection.ignition_ignition_min"),
		WeightPeakScore:          viper.GetFloat64("selection.weight_peak_score"),
		WeightPrecursor:          viper.GetFloat64("selection.weight_precursor"),
		WeightTapeUptake:         viper.GetFloat64("selection.weight_tape_uptake"),
		WeightSpread:             viper.GetFloat64("selection.weight_spread"),
		WeightPeakFrac:           viper.GetFloat64("selection.weight_peak_frac"),
		WeightRotation:           viper.GetFloat64("selection.weight_rotation"),
		HeartbeatBatch:           viper.GetInt("selection.heartbeat_batch"),
		StoplossArmPnlPct:        viper.GetFloat64("selection.stoploss_arm_pnl_pct"),
		StoplossRateDecayMin:     viper.GetFloat64("selection.stoploss_rate_decay_min"),
		StoplossPumpConfirmMin:   viper.GetFloat64("selection.stoploss_pump_confirm_min"),
		StoplossConfirmMarks:     viper.GetInt("selection.stoploss_confirm_marks"),
		StoplossNearPeakFrac:     viper.GetFloat64("selection.stoploss_near_peak_frac"),
		StoplossGivebackPct:      viper.GetFloat64("selection.stoploss_giveback_pct"),
		StoplossGrindVelocityMax: viper.GetFloat64("selection.stoploss_grind_velocity_max"),
		StoplossGrindMinSamples:  viper.GetInt("selection.stoploss_grind_min_samples"),
		RotationCloseBoost:       viper.GetFloat64("selection.rotation_close_boost"),
	}
}
