package audit

/*
catalogMetric is one numeric signal named in SIGNALS.md. The vitality stage
judges this list, not every series the tape happens to emit. Grammar is omitted:
it is a rule trace, not a number that can move. fee_rate is a venue parameter
and is required to be present, not to change.
*/
type catalogMetric struct {
	Origin string
	Name   string
	Static bool
}

func (metric catalogMetric) key() string {
	return metric.Origin + "." + metric.Name
}

func signalCatalog() []catalogMetric {
	return []catalogMetric{
		{Origin: "correlation", Name: "cohort_covariance_score"},
		{Origin: "correlation", Name: "historical_path_distance"},
		{Origin: "correlation", Name: "historical_path_percentile"},
		{Origin: "cvd", Name: "flow_aligned_midpoint_return"},
		{Origin: "cvd", Name: "gross_notional"},
		{Origin: "cvd", Name: "midpoint_log_return"},
		{Origin: "cvd", Name: "signed_net_fraction"},
		{Origin: "depthflow", Name: "book_imbalance"},
		{Origin: "depthflow", Name: "mutation_activity_imbalance"},
		{Origin: "depthflow", Name: "net_displayed_flow:bid"},
		{Origin: "depthflow", Name: "touch_imbalance"},
		{Origin: "depthflow", Name: "touch_notional:ask"},
		{Origin: "depthflow", Name: "touch_notional:bid"},
		{Origin: "hawkes", Name: "arrival_rate:buy"},
		{Origin: "hawkes", Name: "arrival_rate:sell"},
		{Origin: "hawkes", Name: "branching_spectral_radius"},
		{Origin: "hawkes", Name: "conditional_intensity_velocity"},
		{Origin: "leadlag", Name: "best_lag_seconds_mad"},
		{Origin: "leadlag", Name: "best_lag_seconds_median"},
		{Origin: "leadlag", Name: "covariance_score_gain_median"},
		{Origin: "leadlag", Name: "led_peer_share"},
		{Origin: "liquidity", Name: "fee_rate", Static: true},
		{Origin: "liquidity", Name: "friction_hurdle"},
		{Origin: "liquidity", Name: "relative_spread"},
		{Origin: "morphology", Name: "book_shape_distance"},
		{Origin: "morphology", Name: "book_shape_ks"},
		{Origin: "morphology", Name: "concentration:ask"},
		{Origin: "morphology", Name: "concentration:bid"},
		{Origin: "morphology", Name: "entropy:ask"},
		{Origin: "morphology", Name: "entropy:bid"},
		{Origin: "morphology", Name: "morphology_change"},
		{Origin: "pumpdump", Name: "negative_midpoint_return"},
		{Origin: "pumpdump", Name: "positive_midpoint_return"},
		{Origin: "pumpdump", Name: "spread_ratio"},
		{Origin: "pumpdump", Name: "trade_rate"},
		{Origin: "pumpdump", Name: "volume_rate"},
		{Origin: "pumpdump", Name: "volume_ratio"},
		{Origin: "sentiment", Name: "breadth"},
		{Origin: "sentiment", Name: "breadth_velocity"},
		{Origin: "sentiment", Name: "historical_path_distance"},
		{Origin: "sentiment", Name: "historical_path_percentile"},
		{Origin: "sentiment", Name: "opposite_direction_peer_count"},
		{Origin: "sentiment", Name: "same_direction_peer_count"},
		{Origin: "toxicity", Name: "net_withdrawal_rate:bid"},
		{Origin: "toxicity", Name: "retreat_fraction:bid"},
	}
}
