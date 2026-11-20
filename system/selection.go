package system

import (
	"time"

	"github.com/spf13/viper"
)

/*
Selection configures cross-sectional board ranking, capacity limits, and dual
peak / ignition thresholds.
*/
type Selection struct {
	MaxOpenPositions       int
	MaxEntersPerTick       int
	MinBoardScore          float64
	BoardPeerWindow        time.Duration
	MaxAccumPeakFrac       float64
	MaxIgnitionPeakFrac    float64
	IgnitionPrecursorMin   float64
	IgnitionCompressionMin float64
	IgnitionIgnitionMin    float64
	WeightPeakScore        float64
	WeightPrecursor        float64
	WeightTapeUptake       float64
	WeightSpread           float64
	WeightPeakFrac         float64
	// WeightRotation scales calibrated resonance task_skill in board ranking.
	WeightRotation float64
	HeartbeatBatch int
	WeightSync     float64
	WeightSurge    float64
	WeightDepth    float64
	// WeightRushBuy scales buy market-order rush in board scoring.
	WeightRushBuy float64
	// WeightRhythm scales organic vs mechanical tape rhythm in board scoring.
	WeightRhythm float64
}

func NewSelection() *Selection {
	viper.SetDefault("selection.max_open_positions", 18)
	viper.SetDefault("selection.max_enters_per_tick", 2)
	viper.SetDefault("selection.min_board_score", 0.35)
	viper.SetDefault("selection.board_peer_window", 2*time.Second)
	viper.SetDefault("selection.max_accum_peak_frac", 0.85)
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
	viper.SetDefault("selection.weight_sync", 0.45)
	viper.SetDefault("selection.weight_surge", 0.35)
	viper.SetDefault("selection.weight_depth", 0.30)
	viper.SetDefault("selection.weight_rush_buy", 0.55)
	viper.SetDefault("selection.weight_rhythm", 0.25)

	return &Selection{
		MaxOpenPositions:       viper.GetInt("selection.max_open_positions"),
		MaxEntersPerTick:       viper.GetInt("selection.max_enters_per_tick"),
		MinBoardScore:          viper.GetFloat64("selection.min_board_score"),
		BoardPeerWindow:        viper.GetDuration("selection.board_peer_window"),
		MaxAccumPeakFrac:       viper.GetFloat64("selection.max_accum_peak_frac"),
		MaxIgnitionPeakFrac:    viper.GetFloat64("selection.max_ignition_peak_frac"),
		IgnitionPrecursorMin:   viper.GetFloat64("selection.ignition_precursor_min"),
		IgnitionCompressionMin: viper.GetFloat64("selection.ignition_compression_min"),
		IgnitionIgnitionMin:    viper.GetFloat64("selection.ignition_ignition_min"),
		WeightPeakScore:        viper.GetFloat64("selection.weight_peak_score"),
		WeightPrecursor:        viper.GetFloat64("selection.weight_precursor"),
		WeightTapeUptake:       viper.GetFloat64("selection.weight_tape_uptake"),
		WeightSpread:           viper.GetFloat64("selection.weight_spread"),
		WeightPeakFrac:         viper.GetFloat64("selection.weight_peak_frac"),
		WeightRotation:         viper.GetFloat64("selection.weight_rotation"),
		HeartbeatBatch:         viper.GetInt("selection.heartbeat_batch"),
		WeightSync:             viper.GetFloat64("selection.weight_sync"),
		WeightSurge:            viper.GetFloat64("selection.weight_surge"),
		WeightDepth:            viper.GetFloat64("selection.weight_depth"),
		WeightRushBuy:          viper.GetFloat64("selection.weight_rush_buy"),
		WeightRhythm:           viper.GetFloat64("selection.weight_rhythm"),
	}
}
