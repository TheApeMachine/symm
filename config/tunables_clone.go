package config

/*
CloneTunables returns a deep copy so hill-climb mutations do not alias cfg fields.
*/
func CloneTunables(source Tunables) Tunables {
	clone := Tunables{}

	if source.EntryEdgeMultiple != nil {
		value := *source.EntryEdgeMultiple
		clone.EntryEdgeMultiple = &value
	}

	if source.TakeProfitR != nil {
		value := *source.TakeProfitR
		clone.TakeProfitR = &value
	}

	if source.TakeProfitCapture != nil {
		value := *source.TakeProfitCapture
		clone.TakeProfitCapture = &value
	}

	if source.StopVolMultiple != nil {
		value := *source.StopVolMultiple
		clone.StopVolMultiple = &value
	}

	if source.MinExhaustHold != nil {
		value := *source.MinExhaustHold
		clone.MinExhaustHold = &value
	}

	if source.PumpTrailPct != nil {
		value := *source.PumpTrailPct
		clone.PumpTrailPct = &value
	}

	if source.PumpSlowTrailPct != nil {
		value := *source.PumpSlowTrailPct
		clone.PumpSlowTrailPct = &value
	}

	if source.PumpHardStopPct != nil {
		value := *source.PumpHardStopPct
		clone.PumpHardStopPct = &value
	}

	if source.PumpSizeFraction != nil {
		value := *source.PumpSizeFraction
		clone.PumpSizeFraction = &value
	}

	if source.KellyFraction != nil {
		value := *source.KellyFraction
		clone.KellyFraction = &value
	}

	if source.MaxDeployPct != nil {
		value := *source.MaxDeployPct
		clone.MaxDeployPct = &value
	}

	if source.MaxEntrySlippageBPS != nil {
		value := *source.MaxEntrySlippageBPS
		clone.MaxEntrySlippageBPS = &value
	}

	if source.MaxSpreadBPS != nil {
		value := *source.MaxSpreadBPS
		clone.MaxSpreadBPS = &value
	}

	if source.ForwardReturnMinSamples != nil {
		value := *source.ForwardReturnMinSamples
		clone.ForwardReturnMinSamples = &value
	}

	if source.ForwardReturnSignificanceZ != nil {
		value := *source.ForwardReturnSignificanceZ
		clone.ForwardReturnSignificanceZ = &value
	}

	if source.PerspectiveTTL != nil {
		value := *source.PerspectiveTTL
		clone.PerspectiveTTL = &value
	}

	if source.NoiseFloorSNR != nil {
		value := *source.NoiseFloorSNR
		clone.NoiseFloorSNR = &value
	}

	if source.BookDepthLevels != nil {
		value := *source.BookDepthLevels
		clone.BookDepthLevels = &value
	}

	if source.MinCostEUR != nil {
		value := *source.MinCostEUR
		clone.MinCostEUR = &value
	}

	if source.CausalConditionSwitch != nil {
		value := *source.CausalConditionSwitch
		clone.CausalConditionSwitch = &value
	}

	if source.CausalContagionBreak != nil {
		value := *source.CausalContagionBreak
		clone.CausalContagionBreak = &value
	}

	if source.FluidHeightEMAAlpha != nil {
		value := *source.FluidHeightEMAAlpha
		clone.FluidHeightEMAAlpha = &value
	}

	if source.HawkesFitCooldown != nil {
		value := *source.HawkesFitCooldown
		clone.HawkesFitCooldown = &value
	}

	return clone
}
