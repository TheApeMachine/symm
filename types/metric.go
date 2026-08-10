package types

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
	MetricSNR                             MetricType = "snr"
	MetricStrength                        MetricType = "strength"
	MetricValue                           MetricType = "value"
	MetricCategory                        MetricType = "category"
	MetricMidpoint                        MetricType = "midpoint"
	MetricLastPrice                       MetricType = "last_price"
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
	MetricSampleSupport            MetricType = "sample_support"
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
)
