package system

import (
	"time"

	"github.com/spf13/viper"
)

type Risk struct {
	MaxFraction         float64
	TouchDivisor        float64
	MinMoverVolumeUSD   float64
	MinMoverChangePct   float64
	MaxEntrySpreadPct   float64
	MaxEntryOpenDragPct float64
	MinTouchBidCash     float64
	TapeWindow          time.Duration
	MinTapeTrades       int
	MaxAllowedLossPct   float64
	BackingBandFrac     float64
	MinBackingRatio     float64
	// StopPaceHalflife is the TimeElastic memory for mark-arrival speed.
	StopPaceHalflife time.Duration
	// StopPaceEpsilon floors the TimeElastic baseline denominator.
	StopPaceEpsilon float64
}

func NewRisk() *Risk {
	viper.SetDefault("risk.max_fraction", 0.20)
	viper.SetDefault("risk.touch_divisor", 3)
	viper.SetDefault("risk.min_mover_volume_usd", 100_000.0)
	viper.SetDefault("risk.min_mover_change_pct", 0.5)
	viper.SetDefault("risk.max_entry_spread_pct", 2.0)
	viper.SetDefault("risk.max_entry_open_drag_pct", 2.5)
	viper.SetDefault("risk.min_touch_bid_cash", 100.0)
	viper.SetDefault("risk.tape_window", 10*time.Minute)
	viper.SetDefault("risk.min_tape_trades", 8)
	viper.SetDefault("risk.max_allowed_loss_pct", 2.5)
	viper.SetDefault("risk.backing_band_frac", 0.01)
	viper.SetDefault("risk.min_backing_ratio", 0.6)
	viper.SetDefault("risk.stop_pace_halflife", 2*time.Second)
	viper.SetDefault("risk.stop_pace_epsilon", 1e-6)

	return &Risk{
		MaxFraction:         viper.GetFloat64("risk.max_fraction"),
		TouchDivisor:        float64(viper.GetInt("risk.touch_divisor")),
		MinMoverVolumeUSD:   viper.GetFloat64("risk.min_mover_volume_usd"),
		MinMoverChangePct:   viper.GetFloat64("risk.min_mover_change_pct"),
		MaxEntrySpreadPct:   viper.GetFloat64("risk.max_entry_spread_pct"),
		MaxEntryOpenDragPct: viper.GetFloat64("risk.max_entry_open_drag_pct"),
		MinTouchBidCash:     viper.GetFloat64("risk.min_touch_bid_cash"),
		TapeWindow:          viper.GetDuration("risk.tape_window"),
		MinTapeTrades:       viper.GetInt("risk.min_tape_trades"),
		MaxAllowedLossPct:   viper.GetFloat64("risk.max_allowed_loss_pct"),
		BackingBandFrac:     viper.GetFloat64("risk.backing_band_frac"),
		MinBackingRatio:     viper.GetFloat64("risk.min_backing_ratio"),
		StopPaceHalflife:    viper.GetDuration("risk.stop_pace_halflife"),
		StopPaceEpsilon:     viper.GetFloat64("risk.stop_pace_epsilon"),
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

	setFloat(&risk.MinMoverVolumeUSD, "min_mover_volume_usd")
	setFloat(&risk.MinMoverChangePct, "min_mover_change_pct")
	setFloat(&risk.MaxEntrySpreadPct, "max_entry_spread_pct")
	setFloat(&risk.MinTouchBidCash, "min_touch_bid_cash")
	setFloat(&risk.MinBackingRatio, "min_backing_ratio")
	setFloat(&risk.MaxEntryOpenDragPct, "max_entry_open_drag_pct")
	setInt(&risk.MinTapeTrades, "min_tape_trades")
}
