package config

import (
	"os"
	"path/filepath"
	"time"
)

/*
Tunables is the persisted subset of Config fields the optimizer mutates.
JSON keys use snake_case; durations are Go duration strings (e.g. "30s").
*/
type Tunables struct {
	EntryEdgeMultiple          *float64       `json:"entry_edge_multiple,omitempty"`
	TakeProfitR                *float64       `json:"take_profit_r,omitempty"`
	TakeProfitCapture          *float64       `json:"take_profit_capture,omitempty"`
	StopVolMultiple            *float64       `json:"stop_vol_multiple,omitempty"`
	MinExhaustHold             *time.Duration `json:"min_exhaust_hold,omitempty"`
	PumpTrailPct               *float64       `json:"pump_trail_pct,omitempty"`
	PumpSlowTrailPct           *float64       `json:"pump_slow_trail_pct,omitempty"`
	PumpHardStopPct            *float64       `json:"pump_hard_stop_pct,omitempty"`
	PumpSizeFraction           *float64       `json:"pump_size_fraction,omitempty"`
	KellyFraction              *float64       `json:"kelly_fraction,omitempty"`
	MaxDeployPct               *float64       `json:"max_deploy_pct,omitempty"`
	MaxEntrySlippageBPS        *float64       `json:"max_entry_slippage_bps,omitempty"`
	MaxSpreadBPS               *float64       `json:"max_spread_bps,omitempty"`
	ForwardReturnMinSamples    *int           `json:"forward_return_min_samples,omitempty"`
	ForwardReturnSignificanceZ *float64       `json:"forward_return_significance_z,omitempty"`
	PerspectiveTTL             *time.Duration `json:"perspective_ttl,omitempty"`
	BookDepthLevels            *int           `json:"book_depth_levels,omitempty"`
	MinCostEUR                 *float64       `json:"min_cost_eur,omitempty"`
	CausalConditionSwitch      *float64       `json:"causal_condition_switch,omitempty"`
	CausalContagionBreak       *float64       `json:"causal_contagion_break,omitempty"`
	FluidHeightEMAAlpha        *float64       `json:"fluid_height_ema_alpha,omitempty"`
	HawkesFitCooldown          *time.Duration `json:"hawkes_fit_cooldown,omitempty"`
}

/*
Apply merges tunables into cfg without touching unset fields.
*/
func (tunables *Tunables) Apply(cfg *Config) {
	if tunables == nil || cfg == nil {
		return
	}

	if tunables.EntryEdgeMultiple != nil {
		cfg.EntryEdgeMultiple = *tunables.EntryEdgeMultiple
	}

	if tunables.TakeProfitR != nil {
		cfg.TakeProfitR = *tunables.TakeProfitR
	}

	if tunables.TakeProfitCapture != nil {
		cfg.TakeProfitCapture = *tunables.TakeProfitCapture
	}

	if tunables.StopVolMultiple != nil {
		cfg.StopVolMultiple = *tunables.StopVolMultiple
	}

	if tunables.MinExhaustHold != nil {
		cfg.MinExhaustHold = *tunables.MinExhaustHold
	}

	if tunables.PumpTrailPct != nil {
		cfg.PumpTrailPct = *tunables.PumpTrailPct
	}

	if tunables.PumpSlowTrailPct != nil {
		cfg.PumpSlowTrailPct = *tunables.PumpSlowTrailPct
	}

	if tunables.PumpHardStopPct != nil {
		cfg.PumpHardStopPct = *tunables.PumpHardStopPct
	}

	if tunables.PumpSizeFraction != nil {
		cfg.PumpSizeFraction = *tunables.PumpSizeFraction
	}

	if tunables.KellyFraction != nil {
		cfg.KellyFraction = *tunables.KellyFraction
	}

	if tunables.MaxDeployPct != nil {
		cfg.MaxDeployPct = *tunables.MaxDeployPct
	}

	if tunables.MaxEntrySlippageBPS != nil {
		cfg.MaxEntrySlippageBPS = *tunables.MaxEntrySlippageBPS
	}

	if tunables.MaxSpreadBPS != nil {
		cfg.MaxSpreadBPS = *tunables.MaxSpreadBPS
	}

	if tunables.ForwardReturnMinSamples != nil {
		cfg.ForwardReturnMinSamples = *tunables.ForwardReturnMinSamples
	}

	if tunables.ForwardReturnSignificanceZ != nil {
		cfg.ForwardReturnSignificanceZ = *tunables.ForwardReturnSignificanceZ
	}

	if tunables.PerspectiveTTL != nil {
		cfg.PerspectiveTTL = *tunables.PerspectiveTTL
	}

	if tunables.BookDepthLevels != nil {
		cfg.BookDepthLevels = *tunables.BookDepthLevels
	}

	if tunables.MinCostEUR != nil {
		cfg.MinCostEUR = *tunables.MinCostEUR
	}

	if tunables.CausalConditionSwitch != nil {
		cfg.CausalConditionSwitch = *tunables.CausalConditionSwitch
	}

	if tunables.CausalContagionBreak != nil {
		cfg.CausalContagionBreak = *tunables.CausalContagionBreak
	}

	if tunables.FluidHeightEMAAlpha != nil {
		cfg.FluidHeightEMAAlpha = *tunables.FluidHeightEMAAlpha
	}

	if tunables.HawkesFitCooldown != nil {
		cfg.HawkesFitCooldown = *tunables.HawkesFitCooldown
	}
}

/*
ExtractTunables snapshots the tunable fields from cfg.
*/
func ExtractTunables(cfg *Config) Tunables {
	entryEdge := cfg.EntryEdgeMultiple
	takeProfitR := cfg.TakeProfitR
	takeProfitCapture := cfg.TakeProfitCapture
	stopVol := cfg.StopVolMultiple
	minExhaust := cfg.MinExhaustHold
	pumpTrail := cfg.PumpTrailPct
	pumpSlowTrail := cfg.PumpSlowTrailPct
	pumpHardStop := cfg.PumpHardStopPct
	pumpSize := cfg.PumpSizeFraction
	kelly := cfg.KellyFraction
	maxDeploy := cfg.MaxDeployPct
	maxSlippage := cfg.MaxEntrySlippageBPS
	maxSpread := cfg.MaxSpreadBPS
	forwardMin := cfg.ForwardReturnMinSamples
	forwardZ := cfg.ForwardReturnSignificanceZ
	perspectiveTTL := cfg.PerspectiveTTL
	bookDepth := cfg.BookDepthLevels
	minCost := cfg.MinCostEUR
	causalSwitch := cfg.CausalConditionSwitch
	causalBreak := cfg.CausalContagionBreak
	fluidAlpha := cfg.FluidHeightEMAAlpha
	hawkesCooldown := cfg.HawkesFitCooldown

	return Tunables{
		EntryEdgeMultiple:          &entryEdge,
		TakeProfitR:                &takeProfitR,
		TakeProfitCapture:          &takeProfitCapture,
		StopVolMultiple:            &stopVol,
		MinExhaustHold:             &minExhaust,
		PumpTrailPct:               &pumpTrail,
		PumpSlowTrailPct:           &pumpSlowTrail,
		PumpHardStopPct:            &pumpHardStop,
		PumpSizeFraction:           &pumpSize,
		KellyFraction:              &kelly,
		MaxDeployPct:               &maxDeploy,
		MaxEntrySlippageBPS:        &maxSlippage,
		MaxSpreadBPS:               &maxSpread,
		ForwardReturnMinSamples:    &forwardMin,
		ForwardReturnSignificanceZ: &forwardZ,
		PerspectiveTTL:             &perspectiveTTL,
		BookDepthLevels:            &bookDepth,
		MinCostEUR:                 &minCost,
		CausalConditionSwitch:      &causalSwitch,
		CausalContagionBreak:       &causalBreak,
		FluidHeightEMAAlpha:        &fluidAlpha,
		HawkesFitCooldown:          &hawkesCooldown,
	}
}

/*
DefaultTunedPath is where successful tune runs persist settings for startup load.
*/
func DefaultTunedPath() string {
	return defaultTunedFile
}

func LoadTunablesFile(path string, cfg *Config) error {
	payload, err := os.ReadFile(path)

	if err != nil {
		return err
	}

	tunables, err := decodeTunablesFile(payload)

	if err != nil {
		return err
	}

	tunables.Apply(cfg)

	return nil
}

/*
SaveTunablesFile writes tunables extracted from cfg to path.
*/
func SaveTunablesFile(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	payload, err := encodeTunablesFile(ExtractTunables(cfg))

	if err != nil {
		return err
	}

	return os.WriteFile(path, payload, 0o644)
}
