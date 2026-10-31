package system

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
)

type Risk struct {
	MaxFraction          *decimal.Decimal
	TouchDivisor         *decimal.Decimal
	MinMoverVolumeUSD    float64
	MinMoverChangePct    float64
	MaxEntrySpreadPct    float64
	MaxEntryOpenDragPct  float64
	MinTouchBidCash      float64
	TapeWindow           time.Duration
	MinTapeTrades        int
	MaxAllowedLossPct    float64
	TrailPeakArmPct      float64
	TrailBaseGivebackPct float64
	TrailMinGivebackPct  float64
	BackingBandFrac      float64
	MinBackingRatio      float64
}

func NewRisk() *Risk {
	viper.SetDefault("risk.max_fraction", 0.20)
	viper.SetDefault("risk.touch_divisor", 3)
	viper.SetDefault("risk.min_mover_volume_usd", 100_000.0)
	viper.SetDefault("risk.min_mover_change_pct", 0.5)
	viper.SetDefault("risk.max_entry_spread_pct", 2.0)
	viper.SetDefault("risk.max_entry_open_drag_pct", 2.0)
	viper.SetDefault("risk.min_touch_bid_cash", 100.0)
	viper.SetDefault("risk.tape_window", 10*time.Minute)
	viper.SetDefault("risk.min_tape_trades", 8)
	viper.SetDefault("risk.max_allowed_loss_pct", 2.0)
	viper.SetDefault("risk.trail_peak_arm_pct", 0.5)
	viper.SetDefault("risk.trail_base_giveback_pct", 1.5)
	viper.SetDefault("risk.trail_min_giveback_pct", 0.35)
	viper.SetDefault("risk.backing_band_frac", 0.01)
	viper.SetDefault("risk.min_backing_ratio", 0.6)

	return &Risk{
		MaxFraction:          decimal.NewFromFloat64(viper.GetFloat64("risk.max_fraction")),
		TouchDivisor:         decimal.NewFromInt64(int64(viper.GetInt("risk.touch_divisor"))),
		MinMoverVolumeUSD:    viper.GetFloat64("risk.min_mover_volume_usd"),
		MinMoverChangePct:    viper.GetFloat64("risk.min_mover_change_pct"),
		MaxEntrySpreadPct:    viper.GetFloat64("risk.max_entry_spread_pct"),
		MaxEntryOpenDragPct:  viper.GetFloat64("risk.max_entry_open_drag_pct"),
		MinTouchBidCash:      viper.GetFloat64("risk.min_touch_bid_cash"),
		TapeWindow:           viper.GetDuration("risk.tape_window"),
		MinTapeTrades:        viper.GetInt("risk.min_tape_trades"),
		MaxAllowedLossPct:    viper.GetFloat64("risk.max_allowed_loss_pct"),
		TrailPeakArmPct:      viper.GetFloat64("risk.trail_peak_arm_pct"),
		TrailBaseGivebackPct: viper.GetFloat64("risk.trail_base_giveback_pct"),
		TrailMinGivebackPct:  viper.GetFloat64("risk.trail_min_giveback_pct"),
		BackingBandFrac:      viper.GetFloat64("risk.backing_band_frac"),
		MinBackingRatio:      viper.GetFloat64("risk.min_backing_ratio"),
	}
}
