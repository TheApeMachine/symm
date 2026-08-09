package types

import coretypes "github.com/theapemachine/symm/types"

/*
Clone detaches every mutable scenario collection so replay identity cannot be
changed through caller-owned symbol pointers, maps, slices, or fault payloads.
*/
func (config ScenarioConfig) Clone() ScenarioConfig {
	clone := config
	clone.Symbols = make([]*Symbol, len(config.Symbols))

	for index, symbol := range config.Symbols {
		if symbol == nil {
			continue
		}

		profile := *symbol
		clone.Symbols[index] = &profile
	}

	clone.Schedule = append([]RegimeTransition(nil), config.Schedule...)
	clone.Profiles = CloneProfiles(config.Profiles)
	clone.Momentum = CloneMomentum(config.Momentum)
	clone.Execution.Outcomes = append([]OrderOutcome(nil), config.Execution.Outcomes...)
	clone.Faults.ChannelLatency = copyLatency(config.Faults.ChannelLatency)
	clone.Faults.RESTLatency = copyLatency(config.Faults.RESTLatency)
	if config.Faults.Rules != nil {
		clone.Faults.Rules = make([]FaultRule, len(config.Faults.Rules))
	}

	for index, rule := range config.Faults.Rules {
		clone.Faults.Rules[index] = rule
		clone.Faults.Rules[index].Payload = append([]byte(nil), rule.Payload...)
	}

	if config.InitialBalances != nil {
		clone.InitialBalances = make(map[string]float64, len(config.InitialBalances))
	}

	for asset, balance := range config.InitialBalances {
		clone.InitialBalances[asset] = balance
	}

	return clone
}

/*
CloneProfiles detaches regime contracts, including their expectation slices.
*/
func CloneProfiles(
	source map[MarketState]RegimeProfile,
) map[MarketState]RegimeProfile {
	if source == nil {
		return nil
	}

	copy := make(map[MarketState]RegimeProfile, len(source))

	for state, profile := range source {
		profile.Precursor.Metrics = append(
			[]PrecursorMetricExpectation(nil), profile.Precursor.Metrics...,
		)
		profile.Precursor.Categories = append(
			[]coretypes.CategoryType(nil), profile.Precursor.Categories...,
		)
		copy[state] = profile
	}

	return copy
}

/*
CloneMomentum detaches the transition-speed contract.
*/
func CloneMomentum(source map[MarketState]float64) map[MarketState]float64 {
	if source == nil {
		return nil
	}

	copy := make(map[MarketState]float64, len(source))

	for state, momentum := range source {
		copy[state] = momentum
	}

	return copy
}

func copyLatency(source map[string]LatencyConfig) map[string]LatencyConfig {
	if source == nil {
		return nil
	}

	copy := make(map[string]LatencyConfig, len(source))

	for identity, latency := range source {
		copy[identity] = latency
	}

	return copy
}
