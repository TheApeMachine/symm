package config

import (
	"encoding/json"
	"time"
)

type tunablesWire struct {
	EntryEdgeMultiple          *float64 `json:"entry_edge_multiple,omitempty"`
	TakeProfitR                *float64 `json:"take_profit_r,omitempty"`
	TakeProfitCapture          *float64 `json:"take_profit_capture,omitempty"`
	StopVolMultiple            *float64 `json:"stop_vol_multiple,omitempty"`
	MinExhaustHold             *string  `json:"min_exhaust_hold,omitempty"`
	PumpTrailPct               *float64 `json:"pump_trail_pct,omitempty"`
	PumpSlowTrailPct           *float64 `json:"pump_slow_trail_pct,omitempty"`
	PumpHardStopPct            *float64 `json:"pump_hard_stop_pct,omitempty"`
	PumpSizeFraction           *float64 `json:"pump_size_fraction,omitempty"`
	KellyFraction              *float64 `json:"kelly_fraction,omitempty"`
	MaxDeployPct               *float64 `json:"max_deploy_pct,omitempty"`
	MaxEntrySlippageBPS        *float64 `json:"max_entry_slippage_bps,omitempty"`
	MaxSpreadBPS               *float64 `json:"max_spread_bps,omitempty"`
	ForwardReturnMinSamples    *int     `json:"forward_return_min_samples,omitempty"`
	ForwardReturnSignificanceZ *float64 `json:"forward_return_significance_z,omitempty"`
	PerspectiveTTL             *string  `json:"perspective_ttl,omitempty"`
	NoiseFloorSNR              *float64 `json:"noise_floor_snr,omitempty"`
	BookDepthLevels            *int     `json:"book_depth_levels,omitempty"`
	MinCostEUR                 *float64 `json:"min_cost_eur,omitempty"`
	CausalConditionSwitch      *float64 `json:"causal_condition_switch,omitempty"`
	CausalContagionBreak       *float64 `json:"causal_contagion_break,omitempty"`
	FluidHeightEMAAlpha        *float64 `json:"fluid_height_ema_alpha,omitempty"`
	HawkesFitCooldown          *string  `json:"hawkes_fit_cooldown,omitempty"`
}

func tunablesToWire(tunables Tunables) tunablesWire {
	wire := tunablesWire{
		EntryEdgeMultiple:          tunables.EntryEdgeMultiple,
		TakeProfitR:                tunables.TakeProfitR,
		TakeProfitCapture:          tunables.TakeProfitCapture,
		StopVolMultiple:            tunables.StopVolMultiple,
		PumpTrailPct:               tunables.PumpTrailPct,
		PumpSlowTrailPct:           tunables.PumpSlowTrailPct,
		PumpHardStopPct:            tunables.PumpHardStopPct,
		PumpSizeFraction:           tunables.PumpSizeFraction,
		KellyFraction:              tunables.KellyFraction,
		MaxDeployPct:               tunables.MaxDeployPct,
		MaxEntrySlippageBPS:        tunables.MaxEntrySlippageBPS,
		MaxSpreadBPS:               tunables.MaxSpreadBPS,
		ForwardReturnMinSamples:    tunables.ForwardReturnMinSamples,
		ForwardReturnSignificanceZ: tunables.ForwardReturnSignificanceZ,
		NoiseFloorSNR:              tunables.NoiseFloorSNR,
		BookDepthLevels:            tunables.BookDepthLevels,
		MinCostEUR:                 tunables.MinCostEUR,
		CausalConditionSwitch:      tunables.CausalConditionSwitch,
		CausalContagionBreak:       tunables.CausalContagionBreak,
		FluidHeightEMAAlpha:        tunables.FluidHeightEMAAlpha,
	}

	if tunables.MinExhaustHold != nil {
		value := tunables.MinExhaustHold.String()
		wire.MinExhaustHold = &value
	}

	if tunables.PerspectiveTTL != nil {
		value := tunables.PerspectiveTTL.String()
		wire.PerspectiveTTL = &value
	}

	if tunables.HawkesFitCooldown != nil {
		value := tunables.HawkesFitCooldown.String()
		wire.HawkesFitCooldown = &value
	}

	return wire
}

func wireToTunables(wire tunablesWire) (Tunables, error) {
	tunables := Tunables{
		EntryEdgeMultiple:          wire.EntryEdgeMultiple,
		TakeProfitR:                wire.TakeProfitR,
		TakeProfitCapture:          wire.TakeProfitCapture,
		StopVolMultiple:            wire.StopVolMultiple,
		PumpTrailPct:               wire.PumpTrailPct,
		PumpSlowTrailPct:           wire.PumpSlowTrailPct,
		PumpHardStopPct:            wire.PumpHardStopPct,
		PumpSizeFraction:           wire.PumpSizeFraction,
		KellyFraction:              wire.KellyFraction,
		MaxDeployPct:               wire.MaxDeployPct,
		MaxEntrySlippageBPS:        wire.MaxEntrySlippageBPS,
		MaxSpreadBPS:               wire.MaxSpreadBPS,
		ForwardReturnMinSamples:    wire.ForwardReturnMinSamples,
		ForwardReturnSignificanceZ: wire.ForwardReturnSignificanceZ,
		NoiseFloorSNR:              wire.NoiseFloorSNR,
		BookDepthLevels:            wire.BookDepthLevels,
		MinCostEUR:                 wire.MinCostEUR,
		CausalConditionSwitch:      wire.CausalConditionSwitch,
		CausalContagionBreak:       wire.CausalContagionBreak,
		FluidHeightEMAAlpha:        wire.FluidHeightEMAAlpha,
	}

	if wire.MinExhaustHold != nil {
		value, err := time.ParseDuration(*wire.MinExhaustHold)

		if err != nil {
			return Tunables{}, err
		}

		tunables.MinExhaustHold = &value
	}

	if wire.PerspectiveTTL != nil {
		value, err := time.ParseDuration(*wire.PerspectiveTTL)

		if err != nil {
			return Tunables{}, err
		}

		tunables.PerspectiveTTL = &value
	}

	if wire.HawkesFitCooldown != nil {
		value, err := time.ParseDuration(*wire.HawkesFitCooldown)

		if err != nil {
			return Tunables{}, err
		}

		tunables.HawkesFitCooldown = &value
	}

	return tunables, nil
}

func decodeTunablesFile(payload []byte) (Tunables, error) {
	var wire tunablesWire

	if err := json.Unmarshal(payload, &wire); err != nil {
		return Tunables{}, err
	}

	return wireToTunables(wire)
}

func encodeTunablesFile(tunables Tunables) ([]byte, error) {
	return json.MarshalIndent(tunablesToWire(tunables), "", "  ")
}
