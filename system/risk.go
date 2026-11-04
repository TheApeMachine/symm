package system

import (
	"time"

	"github.com/spf13/viper"
)

type Risk struct {
	MaxFraction       float64
	TouchDivisor      float64
	MinMoverVolumeUSD float64
	MinMoverChangePct float64
	MaxEntrySpreadPct float64
	// MaxEntryPeakFrac is the highest allowed last/24h-range position (0–1).
	// At or above this fraction of (high−low) vetoes entry. <=0 or >=1 disables.
	MaxEntryPeakFrac    float64
	MaxEntryOpenDragPct float64
	MinTouchBidCash     float64
	// TapeWindow is one half of the entry tape comparison.
	TapeWindow time.Duration
	// MinTapeNotionalUSD is the quote-dollar floor for the latest tape window.
	MinTapeNotionalUSD float64
	MaxAllowedLossPct  float64
	// ProfitTargetPct is the peak where the profit trail arms.
	// Give-back shrinks from the hard-loss budget here to zero one target higher.
	ProfitTargetPct float64
	BackingBandFrac float64
	MinBackingRatio float64
	// StopPaceHalflife is the TimeElastic memory for mark-arrival speed.
	StopPaceHalflife time.Duration
	// StopPaceEpsilon floors the TimeElastic baseline denominator.
	StopPaceEpsilon float64
	// StopFlatlineTimeout is the duration in profit without a new peak before taking profit.
	StopFlatlineTimeout time.Duration
	// StopFlatlineMinProfitPct is the profit threshold required to arm the flatline exit.
	StopFlatlineMinProfitPct float64
}

func NewRisk() *Risk {
	viper.SetDefault("risk.max_fraction", 0.20)
	viper.SetDefault("risk.touch_divisor", 3)
	viper.SetDefault("risk.min_mover_volume_usd", 100_000.0)
	viper.SetDefault("risk.min_mover_change_pct", 0.5)
	viper.SetDefault("risk.max_entry_spread_pct", 2.0)
	viper.SetDefault("risk.max_entry_peak_frac", 0.90)
	viper.SetDefault("risk.max_entry_open_drag_pct", 2.5)
	viper.SetDefault("risk.min_touch_bid_cash", 100.0)
	viper.SetDefault("risk.tape_window", 10*time.Minute)
	viper.SetDefault("risk.min_tape_notional_usd", 100.0)
	viper.SetDefault("risk.max_allowed_loss_pct", 2.5)
	viper.SetDefault("risk.profit_target_pct", 10.0)
	viper.SetDefault("risk.backing_band_frac", 0.01)
	viper.SetDefault("risk.min_backing_ratio", 0.6)
	viper.SetDefault("risk.stop_pace_halflife", 2*time.Second)
	viper.SetDefault("risk.stop_pace_epsilon", 1e-6)
	viper.SetDefault("risk.stop_flatline_timeout", 15*time.Minute)
	viper.SetDefault("risk.stop_flatline_min_profit_pct", 1.0)

	return &Risk{
		MaxFraction:              viper.GetFloat64("risk.max_fraction"),
		TouchDivisor:             float64(viper.GetInt("risk.touch_divisor")),
		MinMoverVolumeUSD:        viper.GetFloat64("risk.min_mover_volume_usd"),
		MinMoverChangePct:        viper.GetFloat64("risk.min_mover_change_pct"),
		MaxEntrySpreadPct:        viper.GetFloat64("risk.max_entry_spread_pct"),
		MaxEntryPeakFrac:         viper.GetFloat64("risk.max_entry_peak_frac"),
		MaxEntryOpenDragPct:      viper.GetFloat64("risk.max_entry_open_drag_pct"),
		MinTouchBidCash:          viper.GetFloat64("risk.min_touch_bid_cash"),
		TapeWindow:               viper.GetDuration("risk.tape_window"),
		MinTapeNotionalUSD:       viper.GetFloat64("risk.min_tape_notional_usd"),
		MaxAllowedLossPct:        viper.GetFloat64("risk.max_allowed_loss_pct"),
		ProfitTargetPct:          viper.GetFloat64("risk.profit_target_pct"),
		BackingBandFrac:          viper.GetFloat64("risk.backing_band_frac"),
		MinBackingRatio:          viper.GetFloat64("risk.min_backing_ratio"),
		StopPaceHalflife:         viper.GetDuration("risk.stop_pace_halflife"),
		StopPaceEpsilon:          viper.GetFloat64("risk.stop_pace_epsilon"),
		StopFlatlineTimeout:      viper.GetDuration("risk.stop_flatline_timeout"),
		StopFlatlineMinProfitPct: viper.GetFloat64("risk.stop_flatline_min_profit_pct"),
	}
}

/*
Apply patches live risk floors from a dashboard gate-lab command.
Unknown keys are ignored; only finite values land.
*/
func (risk *Risk) Apply(patch map[string]any) {
	if risk == nil || len(patch) == 0 {
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

	setDuration := func(dst *time.Duration, key string) {
		raw, ok := patch[key]
		if !ok || dst == nil {
			return
		}

		switch value := raw.(type) {
		case time.Duration:
			*dst = value
		case string:
			if d, err := time.ParseDuration(value); err == nil {
				*dst = d
			}
		case float64:
			*dst = time.Duration(value)
		case int:
			*dst = time.Duration(value)
		case int64:
			*dst = time.Duration(value)
		}
	}

	setFloat(&risk.MinMoverVolumeUSD, "min_mover_volume_usd")
	setFloat(&risk.MinMoverChangePct, "min_mover_change_pct")
	setFloat(&risk.MaxEntrySpreadPct, "max_entry_spread_pct")
	setFloat(&risk.MaxEntryPeakFrac, "max_entry_peak_frac")
	setFloat(&risk.MinTouchBidCash, "min_touch_bid_cash")
	setFloat(&risk.MinBackingRatio, "min_backing_ratio")
	setFloat(&risk.MaxEntryOpenDragPct, "max_entry_open_drag_pct")
	setFloat(&risk.MinTapeNotionalUSD, "min_tape_notional_usd")
	setFloat(&risk.StopFlatlineMinProfitPct, "stop_flatline_min_profit_pct")
	setDuration(&risk.StopFlatlineTimeout, "stop_flatline_timeout")
}
