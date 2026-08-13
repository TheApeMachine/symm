package types

import "math"

/*
MetricType identifies the numerical quantity a signal measured. It keeps
measurement identity independent of the signal implementation that produced
the value, which lets logic compare compatible evidence without interpreting a
source name as a market state.
*/
type MetricType string

const (
	MetricFit                             MetricType = "fit"
	MetricHawkesPoissonLogLikelihoodDelta MetricType = "hawkes_poisson_likelihood_delta"
	MetricCrossSelfLogLikelihoodDelta     MetricType = "cross_self_likelihood_delta"
	MetricImmediateBuyOffspring           MetricType = "immediate_buy_offspring"
	MetricImmediateSellOffspring          MetricType = "immediate_sell_offspring"
	MetricTotalBuyDescendants             MetricType = "total_buy_descendants"
	MetricTotalSellDescendants            MetricType = "total_sell_descendants"
	MetricBuyArrivalRate                  MetricType = "buy_arrival_rate"
	MetricSellArrivalRate                 MetricType = "sell_arrival_rate"
	MetricEventCount                      MetricType = "event_count"
	MetricArrivalRate                     MetricType = "arrival_rate"
	MetricConditionalIntensity            MetricType = "conditional_intensity"
	MetricBaselineIntensity               MetricType = "baseline_intensity"
	MetricExcitationAmplitude             MetricType = "excitation_amplitude"
	MetricDecayRate                       MetricType = "decay_rate"
	MetricKernelMemory                    MetricType = "kernel_memory"
	MetricSpectralRadius                  MetricType = "spectral_radius"
	MetricHawkesPoissonDelta              MetricType = "hawkes_poisson_likelihood_delta"
	MetricCrossSelfDelta                  MetricType = "cross_self_likelihood_delta"
	MetricImmediateOffspring              MetricType = "immediate_expected_offspring"
	MetricTotalDescendants                MetricType = "expected_total_descendants"
	MetricRVOL                            MetricType = "rvol"
	MetricPrecursor                       MetricType = "precursor"
	MetricSpread                          MetricType = "spread"
	MetricCompression                     MetricType = "compression"
	MetricIgnition                        MetricType = "ignition"
	MetricTrend                           MetricType = "trend"
	MetricExhaustion                      MetricType = "exhaustion"
	MetricHypothesisSeparation            MetricType = "hypothesis_separation"
	MetricStrength                        MetricType = "strength"
	MetricValue                           MetricType = "value"
	MetricCategory                        MetricType = "category"
	MetricMidpoint                        MetricType = "midpoint"
	MetricLastPrice                       MetricType = "last_price"
	MetricPeerLastPrice                   MetricType = "peer_last_price"
	MetricTradePrice                      MetricType = "trade_price"
	MetricTradeQuantity                   MetricType = "trade_quantity"
	MetricVWAP                            MetricType = "vwap"
	MetricReportedVolume                  MetricType = "reported_volume"
	MetricResonanceEnergy                 MetricType = "resonance_energy"
	MetricResonanceSurprise               MetricType = "resonance_surprise"

	// exhaust (microstructure decay)
	MetricMechanical MetricType = "mechanical"
	MetricThermal    MetricType = "thermal"
	MetricFragile    MetricType = "fragile"
	MetricReversal   MetricType = "reversal"
	MetricUrgency    MetricType = "urgency"

	// depthflow (touch-level book imbalance)
	MetricLoadedScore  MetricType = "loaded_score"
	MetricSpoofScore   MetricType = "spoof_score"
	MetricThinScore    MetricType = "thin_score"
	MetricNeutralScore MetricType = "neutral_score"

	// cvd (signed aggressor flow)
	MetricAbsorption  MetricType = "absorption"
	MetricDrive       MetricType = "drive"
	MetricBalance     MetricType = "balance"
	MetricStarvation  MetricType = "starvation"
	MetricNet         MetricType = "net"
	MetricNetFraction MetricType = "net_fraction"

	// correlation and leadlag (cohort relation)
	MetricCorrelation              MetricType = "correlation"
	MetricSigned                   MetricType = "signed"
	MetricRelativeEnergy           MetricType = "relative_energy"
	MetricHerdScore                MetricType = "herd_score"
	MetricAlphaScore               MetricType = "alpha_score"
	MetricNoiseScore               MetricType = "noise_score"
	MetricStressScore              MetricType = "stress_score"
	MetricSignedCorrelation        MetricType = "signed_correlation"
	MetricSignedContempCorrelation MetricType = "signed_contemp_correlation"
	MetricSignedLagCorrelation     MetricType = "signed_lag_correlation"
	MetricLagFraction              MetricType = "lag_fraction"
	MetricSignedLagDirection       MetricType = "signed_lag_direction"
	MetricSampleCount              MetricType = "sample_count"
	MetricInefficient              MetricType = "inefficient"
	MetricSync                     MetricType = "sync"
	MetricDecoupled                MetricType = "decoupled"
	MetricStall                    MetricType = "stall"

	// sentiment (breadth and leadership)
	MetricChange         MetricType = "change"
	MetricBreadth        MetricType = "breadth"
	MetricLeaderStrength MetricType = "leader_strength"
	MetricLeaderEvidence MetricType = "leader_evidence"
	MetricRelativeLead   MetricType = "relative_lead"
	MetricSurgeScore     MetricType = "surge_score"
	MetricDivergentScore MetricType = "divergent_score"
	MetricSlumpScore     MetricType = "slump_score"

	// liquidity (reported turnover and executable touch-depth scarcity)
	MetricScarcityScore                MetricType = "scarcity_score"
	MetricReportedVolumeNotional       MetricType = "reported_volume_notional"
	MetricReportedVolumeNotionalMedian MetricType = "reported_volume_notional_median"
	MetricExecutableTouchDepth         MetricType = "executable_touch_depth"
	MetricExecutableTouchDepthMedian   MetricType = "executable_touch_depth_median"
	MetricRelativeTouchDepth           MetricType = "relative_touch_depth"

	// toxicity (level3 touch liquidity honesty)
	MetricTouchQuantity      MetricType = "touch_quantity"
	MetricBestPrice          MetricType = "best_price"
	MetricTradeVolume        MetricType = "trade_volume"
	MetricFillVolume         MetricType = "fill_volume"
	MetricRetreatingQuantity MetricType = "retreating_quantity"
	MetricCancelledQuantity  MetricType = "cancelled_quantity"
	MetricBluffScore         MetricType = "bluff_score"
	MetricVacuumScore        MetricType = "vacuum_score"
	MetricSupportScore       MetricType = "support_score"
)

/*
SignalMetricGroups assigns every metric emitted by each signal to the evidence
group it supports. Competing groups are hypotheses used to measure definition;
context and derived-summary groups remain explicit but do not become noise.
*/
var SignalMetricGroups = map[SourceType]map[string]struct {
	Group    string
	Competes bool
}{
	SourceCorrelation: {
		MetricKey(MetricHypothesisSeparation, SideNone): {"hypothesis_separation", false},
		MetricKey(MetricLastPrice, SideNone):            {"price", false},
		MetricKey(MetricCorrelation, SideNone):          {"cohort_relation", false},
		MetricKey(MetricSigned, SideNone):               {"cohort_relation", false},
		MetricKey(MetricRelativeEnergy, SideNone):       {"energy_scale", false},
		MetricKey(MetricHerdScore, SideNone):            {"herd", true},
		MetricKey(MetricAlphaScore, SideNone):           {"alpha", true},
		MetricKey(MetricNoiseScore, SideNone):           {"noise", true},
		MetricKey(MetricStressScore, SideNone):          {"stress", true},
	},
	SourceCVD: {
		MetricKey(MetricHypothesisSeparation, SideNone): {"hypothesis_separation", false},
		MetricKey(MetricMidpoint, SideNone):             {"market", false},
		MetricKey(MetricTradePrice, SideNone):           {"market", false},
		MetricKey(MetricTradeQuantity, SideNone):        {"market", false},
		MetricKey(MetricAbsorption, SideNone):           {"absorption", true},
		MetricKey(MetricDrive, SideNone):                {"drive", true},
		MetricKey(MetricBalance, SideNone):              {"balance", true},
		MetricKey(MetricStarvation, SideNone):           {"starvation", true},
		MetricKey(MetricStrength, SideNone):             {"summary", false},
		MetricKey(MetricNetFraction, SideNone):          {"flow", false},
		MetricKey(MetricNet, SideNone):                  {"flow", false},
	},
	SourceDepthFlow: {
		MetricKey(MetricHypothesisSeparation, SideNone): {"hypothesis_separation", false},
		MetricKey(MetricLoadedScore, SideNone):          {"loaded", true},
		MetricKey(MetricSpoofScore, SideNone):           {"spoof", true},
		MetricKey(MetricThinScore, SideNone):            {"thin", true},
		MetricKey(MetricNeutralScore, SideNone):         {"neutral", true},
		MetricKey(MetricBestPrice, SideBuy):             {"book", false},
		MetricKey(MetricBestPrice, SideSell):            {"book", false},
		MetricKey(MetricTouchQuantity, SideBuy):         {"book", false},
		MetricKey(MetricTouchQuantity, SideSell):        {"book", false},
		MetricKey(MetricMidpoint, SideNone):             {"book", false},
		MetricKey(MetricTradePrice, SideNone):           {"trade", false},
		MetricKey(MetricTradeQuantity, SideNone):        {"trade", false},
	},
	SourceExhaustion: {
		MetricKey(MetricHypothesisSeparation, SideNone): {"hypothesis_separation", false},
		MetricKey(MetricMechanical, SideBuy):            {"long_mechanical", true},
		MetricKey(MetricThermal, SideBuy):               {"long_thermal", true},
		MetricKey(MetricFragile, SideBuy):               {"long_fragile", true},
		MetricKey(MetricReversal, SideBuy):              {"long_reversal", true},
		MetricKey(MetricMechanical, SideSell):           {"short_mechanical", true},
		MetricKey(MetricThermal, SideSell):              {"short_thermal", true},
		MetricKey(MetricFragile, SideSell):              {"short_fragile", true},
		MetricKey(MetricReversal, SideSell):             {"short_reversal", true},
		MetricKey(MetricUrgency, SideBuy):               {"long_summary", false},
		MetricKey(MetricStrength, SideBuy):              {"long_summary", false},
		MetricKey(MetricValue, SideBuy):                 {"long_summary", false},
		MetricKey(MetricCategory, SideBuy):              {"long_summary", false},
		MetricKey(MetricUrgency, SideSell):              {"short_summary", false},
		MetricKey(MetricStrength, SideSell):             {"short_summary", false},
		MetricKey(MetricValue, SideSell):                {"short_summary", false},
		MetricKey(MetricCategory, SideSell):             {"short_summary", false},
		MetricKey(MetricBestPrice, SideBuy):             {"book", false},
		MetricKey(MetricBestPrice, SideSell):            {"book", false},
		MetricKey(MetricTouchQuantity, SideBuy):         {"book", false},
		MetricKey(MetricTouchQuantity, SideSell):        {"book", false},
		MetricKey(MetricMidpoint, SideNone):             {"book", false},
		MetricKey(MetricTradePrice, SideNone):           {"trade", false},
		MetricKey(MetricTradeQuantity, SideNone):        {"trade", false},
	},
	SourceHawkes: {
		MetricKey(MetricHypothesisSeparation, SideNone):      {"hypothesis_separation", false},
		MetricKey(MetricEventCount, SideNone):                {"observation", false},
		MetricKey(MetricEventCount, SideBuy):                 {"buy_process", true},
		MetricKey(MetricEventCount, SideSell):                {"sell_process", true},
		MetricKey(MetricArrivalRate, SideBuy):                {"buy_process", true},
		MetricKey(MetricArrivalRate, SideSell):               {"sell_process", true},
		MetricKey(MetricConditionalIntensity, SideBuy):       {"buy_process", true},
		MetricKey(MetricConditionalIntensity, SideSell):      {"sell_process", true},
		MetricKey(MetricBaselineIntensity, SideBuy):          {"buy_process", true},
		MetricKey(MetricBaselineIntensity, SideSell):         {"sell_process", true},
		MetricKey(MetricExcitationAmplitude, SideBuyToBuy):   {"buy_process", true},
		MetricKey(MetricExcitationAmplitude, SideBuyToSell):  {"buy_process", true},
		MetricKey(MetricExcitationAmplitude, SideSellToBuy):  {"sell_process", true},
		MetricKey(MetricExcitationAmplitude, SideSellToSell): {"sell_process", true},
		MetricKey(MetricDecayRate, SideNone):                 {"kernel", false},
		MetricKey(MetricKernelMemory, SideNone):              {"kernel", false},
		MetricKey(MetricSpectralRadius, SideNone):            {"stability", false},
		MetricKey(MetricHawkesPoissonDelta, SideNone):        {"fit", false},
		MetricKey(MetricCrossSelfDelta, SideNone):            {"fit", false},
		MetricKey(MetricImmediateOffspring, SideBuy):         {"buy_process", true},
		MetricKey(MetricImmediateOffspring, SideSell):        {"sell_process", true},
		MetricKey(MetricTotalDescendants, SideBuy):           {"buy_process", true},
		MetricKey(MetricTotalDescendants, SideSell):          {"sell_process", true},
	},
	SourceLeadLag: {
		MetricKey(MetricHypothesisSeparation, SideNone):     {"hypothesis_separation", false},
		MetricKey(MetricLastPrice, SideNone):                {"price", false},
		MetricKey(MetricPeerLastPrice, SideNone):            {"peer_price", false},
		MetricKey(MetricCorrelation, SideNone):              {"relation", false},
		MetricKey(MetricSignedCorrelation, SideNone):        {"relation", false},
		MetricKey(MetricSignedContempCorrelation, SideNone): {"relation", false},
		MetricKey(MetricSignedLagCorrelation, SideNone):     {"relation", false},
		MetricKey(MetricLagFraction, SideNone):              {"lag", false},
		MetricKey(MetricSignedLagDirection, SideNone):       {"lag", false},
		MetricKey(MetricSampleCount, SideNone):              {"support", false},
		MetricKey(MetricInefficient, SideNone):              {"inefficient", true},
		MetricKey(MetricSync, SideNone):                     {"sync", true},
		MetricKey(MetricDecoupled, SideNone):                {"decoupled", true},
		MetricKey(MetricStall, SideNone):                    {"stall", true},
		MetricKey(MetricStrength, SideNone):                 {"summary", false},
	},
	SourceLiquidity: {
		MetricKey(MetricHypothesisSeparation, SideNone):         {"hypothesis_separation", false},
		MetricKey(MetricBestPrice, SideBuy):                     {"market", false},
		MetricKey(MetricBestPrice, SideSell):                    {"market", false},
		MetricKey(MetricTouchQuantity, SideBuy):                 {"market", false},
		MetricKey(MetricTouchQuantity, SideSell):                {"market", false},
		MetricKey(MetricMidpoint, SideNone):                     {"market", false},
		MetricKey(MetricVWAP, SideNone):                         {"market", false},
		MetricKey(MetricReportedVolume, SideNone):               {"market", false},
		MetricKey(MetricExecutableTouchDepth, SideNone):         {"available", true},
		MetricKey(MetricRelativeTouchDepth, SideNone):           {"available", true},
		MetricKey(MetricReportedVolumeNotional, SideNone):       {"available", true},
		MetricKey(MetricScarcityScore, SideNone):                {"scarce", true},
		MetricKey(MetricExecutableTouchDepthMedian, SideNone):   {"cohort_scale", false},
		MetricKey(MetricReportedVolumeNotionalMedian, SideNone): {"cohort_scale", false},
	},
	SourcePumpDump: {
		MetricKey(MetricHypothesisSeparation, SideNone): {"hypothesis_separation", false},
		MetricKey(MetricBestPrice, SideBuy):             {"market", false},
		MetricKey(MetricBestPrice, SideSell):            {"market", false},
		MetricKey(MetricMidpoint, SideNone):             {"market", false},
		MetricKey(MetricTradePrice, SideNone):           {"market", false},
		MetricKey(MetricTradeQuantity, SideNone):        {"market", false},
		MetricKey(MetricRVOL, SideNone):                 {"volume_clock", false},
		MetricKey(MetricSpread, SideNone):               {"market", false},
		MetricKey(MetricPrecursor, SideNone):            {"legacy_summary", false},
		MetricKey(MetricCompression, SideNone):          {"legacy_summary", false},
		MetricKey(MetricIgnition, SideNone):             {"legacy_summary", false},
		MetricKey(MetricTrend, SideNone):                {"legacy_summary", false},
		MetricKey(MetricExhaustion, SideNone):           {"legacy_summary", false},
		MetricKey(MetricStrength, SideNone):             {"legacy_summary", false},
		MetricKey(MetricPrecursor, SideBuy):             {"buy_input", false},
		MetricKey(MetricCompression, SideBuy):           {"buy_compression", true},
		MetricKey(MetricIgnition, SideBuy):              {"buy_ignition", true},
		MetricKey(MetricTrend, SideBuy):                 {"buy_trend", true},
		MetricKey(MetricExhaustion, SideBuy):            {"buy_exhaustion", true},
		MetricKey(MetricStrength, SideBuy):              {"buy_summary", false},
		MetricKey(MetricPrecursor, SideSell):            {"sell_input", false},
		MetricKey(MetricCompression, SideSell):          {"sell_compression", true},
		MetricKey(MetricIgnition, SideSell):             {"sell_ignition", true},
		MetricKey(MetricTrend, SideSell):                {"sell_trend", true},
		MetricKey(MetricExhaustion, SideSell):           {"sell_exhaustion", true},
		MetricKey(MetricStrength, SideSell):             {"sell_summary", false},
	},
	SourceSentiment: {
		MetricKey(MetricHypothesisSeparation, SideNone): {"hypothesis_separation", false},
		MetricKey(MetricLastPrice, SideNone):            {"price", false},
		MetricKey(MetricChange, SideNone):               {"return", false},
		MetricKey(MetricBreadth, SideNone):              {"breadth", false},
		MetricKey(MetricLeaderStrength, SideNone):       {"leadership", false},
		MetricKey(MetricLeaderEvidence, SideNone):       {"leadership", false},
		MetricKey(MetricRelativeLead, SideNone):         {"leadership", false},
		MetricKey(MetricSurgeScore, SideNone):           {"surge", true},
		MetricKey(MetricDivergentScore, SideNone):       {"divergence", false},
		MetricKey(MetricSlumpScore, SideNone):           {"slump", true},
		MetricKey(MetricStrength, SideNone):             {"summary", false},
	},
	SourceToxicity: {
		MetricKey(MetricHypothesisSeparation, SideNone): {"hypothesis_separation", false},
		MetricKey(MetricMidpoint, SideNone):             {"market", false},
		MetricKey(MetricTradeVolume, SideNone):          {"market_activity", false},
		MetricKey(MetricFillVolume, SideBuy):            {"execution", true},
		MetricKey(MetricFillVolume, SideSell):           {"execution", true},
		MetricKey(MetricBestPrice, SideBuy):             {"touch", false},
		MetricKey(MetricBestPrice, SideSell):            {"touch", false},
		MetricKey(MetricTouchQuantity, SideBuy):         {"touch", false},
		MetricKey(MetricTouchQuantity, SideSell):        {"touch", false},
		MetricKey(MetricRetreatingQuantity, SideBuy):    {"retreat", true},
		MetricKey(MetricRetreatingQuantity, SideSell):   {"retreat", true},
		MetricKey(MetricCancelledQuantity, SideBuy):     {"cancellation", true},
		MetricKey(MetricCancelledQuantity, SideSell):    {"cancellation", true},
		MetricKey(MetricBluffScore, SideNone):           {"bluff", true},
		MetricKey(MetricVacuumScore, SideNone):          {"vacuum", true},
		MetricKey(MetricSupportScore, SideNone):         {"support", true},
		MetricKey(MetricStrength, SideNone):             {"summary", false},
		MetricKey(MetricValue, SideNone):                {"summary", false},
		MetricKey(MetricCategory, SideNone):             {"summary", false},
	},
}

/*
MeasurementHypothesisSeparation measures how far the strongest hypothesis
stands above all competing hypothesis energy. It is a category margin, not an
estimate of signal-to-noise ratio, sample precision, or model quality.
Supporting metrics are combined by root mean square so a group does not win
merely because it has more metrics. Competing group strengths combine as
root-sum-square alternatives, so every material competitor raises the floor.
*/
func MeasurementHypothesisSeparation(
	source SourceType,
	metrics map[string]MetricSample,
) (float64, bool) {
	mapping, found := SignalMetricGroups[source]

	if !found {
		panic("types: missing signal metric groups for " + string(source))
	}

	groups := make(map[string]struct {
		energy  float64
		support int
	})

	for metricKey, sample := range metrics {
		membership, exists := mapping[metricKey]

		if !exists {
			panic("types: metric has no signal group: " + string(source) + "/" + metricKey)
		}

		if !membership.Competes || sample.Normalized == nil {
			continue
		}

		value := *sample.Normalized

		if value < 0 {
			panic("types: competing metric must be nonnegative: " + string(source) + "/" + metricKey)
		}

		group := groups[membership.Group]
		group.energy += value * value
		group.support++
		groups[membership.Group] = group
	}

	if len(groups) < 2 {
		return 0, false
	}

	winnerGroup := ""
	winnerStrength := 0.0

	for groupName, group := range groups {
		strength := math.Sqrt(group.energy / float64(group.support))

		if winnerGroup == "" || strength > winnerStrength {
			winnerGroup = groupName
			winnerStrength = strength
		}
	}

	noiseEnergy := 0.0

	for groupName, group := range groups {
		if groupName == winnerGroup {
			continue
		}

		strength := math.Sqrt(group.energy / float64(group.support))
		noiseEnergy += strength * strength
	}

	noiseFloor := math.Sqrt(noiseEnergy)

	if winnerStrength == 0 && noiseFloor == 0 {
		return 0, true
	}

	if noiseFloor >= winnerStrength {
		return 0, true
	}

	return (winnerStrength - noiseFloor) / winnerStrength, true
}
