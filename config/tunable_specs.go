package config

import (
	"math/rand"
	"time"
)

/*
Spec describes one tunable numeric field and its search range.
*/
type Spec struct {
	Name string
	Min  float64
	Max  float64
	Step float64
}

/*
TunableSpecs returns optimizer search bounds for persisted fields.
*/
func TunableSpecs() []Spec {
	return []Spec{
		{Name: "entry_edge_multiple", Min: 1.0, Max: 4.0, Step: 0.25},
		{Name: "take_profit_r", Min: 1.0, Max: 4.0, Step: 0.25},
		{Name: "take_profit_capture", Min: 0.4, Max: 0.95, Step: 0.05},
		{Name: "stop_vol_multiple", Min: 4.0, Max: 16.0, Step: 1.0},
		{Name: "pump_trail_pct", Min: 0.04, Max: 0.15, Step: 0.01},
		{Name: "pump_slow_trail_pct", Min: 0.10, Max: 0.30, Step: 0.02},
		{Name: "pump_hard_stop_pct", Min: 0.06, Max: 0.20, Step: 0.01},
		{Name: "pump_size_fraction", Min: 0.10, Max: 0.50, Step: 0.05},
		{Name: "kelly_fraction", Min: 0.10, Max: 1.0, Step: 0.05},
		{Name: "max_deploy_pct", Min: 0.10, Max: 1.0, Step: 0.05},
		{Name: "max_entry_slippage_bps", Min: 20, Max: 120, Step: 10},
		{Name: "max_spread_bps", Min: 10, Max: 100, Step: 5},
		{Name: "forward_return_min_samples", Min: 10, Max: 80, Step: 5},
		{Name: "forward_return_significance_z", Min: 0.5, Max: 3.0, Step: 0.25},
		{Name: "noise_floor_snr", Min: 0.7, Max: 1.5, Step: 0.05},
		{Name: "perspective_ttl_sec", Min: 10, Max: 120, Step: 5},
		{Name: "book_depth_levels", Min: 5, Max: 25, Step: 5},
		{Name: "min_cost_eur", Min: 0.30, Max: 2.0, Step: 0.15},
		{Name: "min_exhaust_hold_sec", Min: 1, Max: 30, Step: 1},
		{Name: "causal_condition_switch", Min: 100, Max: 5000, Step: 100},
		{Name: "causal_contagion_break", Min: 0.5, Max: 0.99, Step: 0.05},
		{Name: "fluid_height_ema_alpha", Min: 0.10, Max: 0.60, Step: 0.05},
		{Name: "hawkes_fit_cooldown_sec", Min: 1, Max: 15, Step: 1},
	}
}

/*
MutateTunables returns a randomized tunables overlay from specs.
*/
func MutateTunables(source *Config, random *rand.Rand) Tunables {
	if source == nil {
		source = NewConfig()
	}

	if random == nil {
		random = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	base := ExtractTunables(source)
	overlay := base

	for _, spec := range TunableSpecs() {
		value := spec.Min + random.Float64()*(spec.Max-spec.Min)

		if spec.Step > 0 {
			value = mathRound(value/spec.Step) * spec.Step
		}

		switch spec.Name {
		case "entry_edge_multiple":
			overlay.EntryEdgeMultiple = &value
		case "take_profit_r":
			overlay.TakeProfitR = &value
		case "take_profit_capture":
			overlay.TakeProfitCapture = &value
		case "stop_vol_multiple":
			overlay.StopVolMultiple = &value
		case "pump_trail_pct":
			overlay.PumpTrailPct = &value
		case "pump_slow_trail_pct":
			overlay.PumpSlowTrailPct = &value
		case "pump_hard_stop_pct":
			overlay.PumpHardStopPct = &value
		case "pump_size_fraction":
			overlay.PumpSizeFraction = &value
		case "kelly_fraction":
			overlay.KellyFraction = &value
		case "max_deploy_pct":
			overlay.MaxDeployPct = &value
		case "max_entry_slippage_bps":
			overlay.MaxEntrySlippageBPS = &value
		case "max_spread_bps":
			overlay.MaxSpreadBPS = &value
		case "forward_return_min_samples":
			samples := int(value)
			overlay.ForwardReturnMinSamples = &samples
		case "forward_return_significance_z":
			overlay.ForwardReturnSignificanceZ = &value
		case "noise_floor_snr":
			overlay.NoiseFloorSNR = &value
		case "perspective_ttl_sec":
			duration := time.Duration(value) * time.Second
			overlay.PerspectiveTTL = &duration
		case "book_depth_levels":
			depth := int(value)
			overlay.BookDepthLevels = &depth
		case "min_cost_eur":
			overlay.MinCostEUR = &value
		case "min_exhaust_hold_sec":
			duration := time.Duration(value) * time.Second
			overlay.MinExhaustHold = &duration
		case "causal_condition_switch":
			overlay.CausalConditionSwitch = &value
		case "causal_contagion_break":
			overlay.CausalContagionBreak = &value
		case "fluid_height_ema_alpha":
			overlay.FluidHeightEMAAlpha = &value
		case "hawkes_fit_cooldown_sec":
			duration := time.Duration(value) * time.Second
			overlay.HawkesFitCooldown = &duration
		}
	}

	return overlay
}

func mathRound(value float64) float64 {
	if value >= 0 {
		return float64(int(value + 0.5))
	}

	return float64(int(value - 0.5))
}
