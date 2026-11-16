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
	BackingBandFrac    float64
	MinBackingRatio    float64
	// StoplossArmPnLPct is the fee-net PnL% floor before path memory may arm.
	StoplossArmPnLPct float64
	// MinImpulseChangePct is legacy deskgate scale; impulse uses envelope.Pumpdump.
	MinImpulseChangePct float64
	// ImpulseWindow is deprecated (volume-clock ignition replaced wall-clock marks).
	ImpulseWindow time.Duration
	// ImpulseShortWindow is deprecated.
	ImpulseShortWindow time.Duration
	// MinImpulseAccelRatio optional Pumpdump ignition/trend floor; <=0 disables.
	MinImpulseAccelRatio float64
	// MinTapeAccelRatio is recent/prior buy-tape; <=0 disables acceleration.
	MinTapeAccelRatio float64
}

func NewRisk() *Risk {
	viper.SetDefault("risk.max_fraction", 0.20)
	viper.SetDefault("risk.touch_divisor", 3)
	viper.SetDefault("risk.min_mover_volume_usd", 10_000.0)
	viper.SetDefault("risk.min_mover_change_pct", 0.5)
	viper.SetDefault("risk.max_entry_spread_pct", 2.0)
	viper.SetDefault("risk.max_entry_peak_frac", 0.85)
	viper.SetDefault("risk.max_entry_open_drag_pct", 2.5)
	viper.SetDefault("risk.min_touch_bid_cash", 100.0)
	viper.SetDefault("risk.tape_window", 10*time.Minute)
	viper.SetDefault("risk.min_tape_notional_usd", 100.0)
	viper.SetDefault("risk.backing_band_frac", 0.01)
	viper.SetDefault("risk.min_backing_ratio", 0.6)
	viper.SetDefault("risk.stoploss_arm_pnl_pct", 1.0)
	viper.SetDefault("risk.min_impulse_change_pct", 0.20)
	viper.SetDefault("risk.impulse_window", 15*time.Minute)
	viper.SetDefault("risk.impulse_short_window", 5*time.Minute)
	viper.SetDefault("risk.min_impulse_accel_ratio", 1.15)
	viper.SetDefault("risk.min_tape_accel_ratio", 1.15)

	return &Risk{
		MaxFraction:         viper.GetFloat64("risk.max_fraction"),
		TouchDivisor:        float64(viper.GetInt("risk.touch_divisor")),
		MinMoverVolumeUSD:   viper.GetFloat64("risk.min_mover_volume_usd"),
		MinMoverChangePct:   viper.GetFloat64("risk.min_mover_change_pct"),
		MaxEntrySpreadPct:   viper.GetFloat64("risk.max_entry_spread_pct"),
		MaxEntryPeakFrac:    viper.GetFloat64("risk.max_entry_peak_frac"),
		MaxEntryOpenDragPct: viper.GetFloat64("risk.max_entry_open_drag_pct"),
		MinTouchBidCash:     viper.GetFloat64("risk.min_touch_bid_cash"),
		TapeWindow:          viper.GetDuration("risk.tape_window"),
		MinTapeNotionalUSD:  viper.GetFloat64("risk.min_tape_notional_usd"),
		BackingBandFrac:     viper.GetFloat64("risk.backing_band_frac"),
		MinBackingRatio:     viper.GetFloat64("risk.min_backing_ratio"),
		StoplossArmPnLPct:     viper.GetFloat64("risk.stoploss_arm_pnl_pct"),
		MinImpulseChangePct:   viper.GetFloat64("risk.min_impulse_change_pct"),
		ImpulseWindow:          viper.GetDuration("risk.impulse_window"),
		ImpulseShortWindow:     viper.GetDuration("risk.impulse_short_window"),
		MinImpulseAccelRatio:   viper.GetFloat64("risk.min_impulse_accel_ratio"),
		MinTapeAccelRatio:      viper.GetFloat64("risk.min_tape_accel_ratio"),
	}
}

/*
Apply patches live risk floors from a dashboard gate-lab command.
Unknown keys are ignored; only finite values land.
*/
func (risk *Risk) Apply(patch map[string]any) {
	if len(patch) == 0 {
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

	setFloat(&risk.MinMoverVolumeUSD, "min_mover_volume_usd")
	setFloat(&risk.MinMoverChangePct, "min_mover_change_pct")
	setFloat(&risk.MaxEntrySpreadPct, "max_entry_spread_pct")
	setFloat(&risk.MaxEntryPeakFrac, "max_entry_peak_frac")
	setFloat(&risk.MinTouchBidCash, "min_touch_bid_cash")
	setFloat(&risk.MinBackingRatio, "min_backing_ratio")
	setFloat(&risk.MaxEntryOpenDragPct, "max_entry_open_drag_pct")
	setFloat(&risk.MinTapeNotionalUSD, "min_tape_notional_usd")
	setFloat(&risk.StoplossArmPnLPct, "stoploss_arm_pnl_pct")
	setFloat(&risk.MinImpulseChangePct, "min_impulse_change_pct")
	setFloat(&risk.MinImpulseAccelRatio, "min_impulse_accel_ratio")
	setFloat(&risk.MinTapeAccelRatio, "min_tape_accel_ratio")
}
