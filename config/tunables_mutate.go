package config

import (
	"fmt"
	"math/rand"
	"time"
)

/*
MutateTunablesNear resamples a few fields around a known-good overlay for hill-climb search.
*/
func MutateTunablesNear(base Tunables, random *rand.Rand) Tunables {
	if random == nil {
		random = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	overlay := CloneTunables(base)
	specs := TunableSpecs()
	mutationCount := 1 + random.Intn(3)

	for mutation := 0; mutation < mutationCount; mutation++ {
		spec := specs[random.Intn(len(specs))]
		applySpecSample(&overlay, spec, random)
	}

	return overlay
}

func applySpecSample(overlay *Tunables, spec Spec, random *rand.Rand) {
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
	default:
		panic(fmt.Sprintf("config: unknown tunable spec %q", spec.Name))
	}
}
