package advisor

const (
	ParticipationName              = "participation"
	participationClock             = "pumpdump/completed_volume_bar_ordinal"
	participationPredictionHorizon = uint64(1)
)

/*
Participation classifies market-wide cohort breadth and leadership dynamics.
*/
type Participation struct {
	Features []*Feature
}

/*
NewParticipation constructs the Participation advisor with competing features
and falsifiable predictions covering broad lift, local leaders, followers, and
isolated idiosyncratic moves.
*/
func NewParticipation() *Participation {
	keys := []string{
		"sentiment/advance_fraction",
		"sentiment/advance_count",
		"sentiment/decline_count",
		"sentiment/decline_fraction",
		"sentiment/same_direction_peer_count",
		"sentiment/same_direction_peer_fraction",
		"sentiment/opposite_direction_peer_count",
		"sentiment/opposite_direction_peer_fraction",
		"sentiment/zero_return_peer_count",
		"sentiment/zero_return_peer_fraction",
		"sentiment/unchanged_count",
		"sentiment/unchanged_fraction",
		"sentiment/directional_participation",
		"sentiment/largest_move_excess",
		"sentiment/largest_move_mad_excess",
		"sentiment/largest_move_share",
		"sentiment/largest_move_share_baseline",
		"sentiment/largest_move_share_zscore",
		"sentiment/largest_move_ratio_baseline",
		"sentiment/largest_move_ratio_zscore",
		"sentiment/largest_move_tie_count",
		"sentiment/magnitude_mad",
		"sentiment/peer_magnitude_mad",
		"sentiment/mean_absolute_return",
		"sentiment/median_absolute_return",
		"sentiment/median_absolute_return_baseline",
		"sentiment/median_absolute_return_ratio",
		"sentiment/median_absolute_return_velocity",
		"sentiment/peer_median_absolute_return",
		"sentiment/median_return_baseline",
		"sentiment/median_return_divergence",
		"sentiment/median_return_velocity",
		"sentiment/breadth_baseline",
		"sentiment/breadth_velocity",
		"sentiment/breadth_divergence",
		"sentiment/return_interquartile_range",
		"sentiment/return_mad",
		"sentiment/rms_return",
		"sentiment/return_dispersion_baseline",
		"sentiment/return_dispersion_velocity",
		"sentiment/directional_agreement",
		"sentiment/directional_consensus",
		"sentiment/breadth",
		"sentiment/breadth_zscore",
		"sentiment/median_return",
		"sentiment/median_return_zscore",
		"sentiment/return_dispersion_ratio",
		"sentiment/return_dispersion_zscore",
		"sentiment/largest_signed_return",
		"sentiment/largest_move_ratio",
		"sentiment/largest_absolute_return",
		"sentiment/return",
		"sentiment/absolute_return",
		"sentiment/asof_age_seconds",
		"sentiment/from_age_seconds",
		"sentiment/max_asof_age_seconds",
		"sentiment/median_asof_age_seconds",
		"sentiment/median_from_age_seconds",
		"sentiment/cohort_member_count",
		"sentiment/valid_member_count",
		"sentiment/excluded_member_count",
		"sentiment/cohort_horizon_seconds",

		"leadlag/best_lag_seconds",
		"leadlag/contemporaneous_correlation",
		"leadlag/best_lag_correlation",
		"leadlag/best_lag_correlation_baseline",
		"leadlag/best_lag_index",
		"leadlag/absolute_correlation_gain",
		"leadlag/correlation_gain_baseline",
		"leadlag/correlation_p_value",
		"leadlag/lag_fraction",
		"leadlag/lag_zscore",
		"leadlag/lag_velocity",
		"leadlag/lag_baseline_seconds",
		"leadlag/lag_noise_scale_seconds",
		"leadlag/lag_search_resolution_seconds",
		"leadlag/lag_search_span",
		"leadlag/search_adjusted_p_value",
		"leadlag/search_count",
		"leadlag/effective_sample_count",
		"leadlag/observation_count",
		"leadlag/overlap_pair_count",
		"leadlag/measured_return_count",
		"leadlag/reference_return_count",
		"leadlag/last_price",
		"leadlag/lag_peak_prominence",
		"leadlag/lag_peak_curvature",
		"leadlag/correlation_gain_velocity",
		"leadlag/lag_divergence_seconds",

		"correlation/signed_correlation",
		"correlation/absolute_correlation",
		"correlation/cohort_signed_correlation",
		"correlation/relative_return_energy",
		"correlation/cohort_correlation_dispersion",
		"correlation/relative_return_energy_zscore",
		"correlation/correlation_zscore",
		"correlation/correlation_baseline",
		"correlation/correlation_velocity",
		"correlation/correlation_p_value",
		"correlation/correlation_standard_error_fisher",
		"correlation/covariance",
		"correlation/cohort_peer_count",
		"correlation/effective_sample_count",
		"correlation/observation_count",
		"correlation/overlap_density",
		"correlation/overlap_pair_count",
		"correlation/peer_return_energy_rate",
		"correlation/relative_cohort_return_energy",
		"correlation/relative_return_energy_baseline",
		"correlation/relative_return_energy_velocity",
		"correlation/return_energy:measured",
		"correlation/return_energy:reference",
		"correlation/return_energy_rate:measured",
		"correlation/return_energy_rate:reference",
		"correlation/shared_time",
		"correlation/last_price",
		"correlation/supported_return_count:measured",
		"correlation/supported_return_count:reference",
		"correlation/cohort_absolute_correlation",
		"correlation/cohort_effective_peer_count",
		"correlation/focal_return_energy_rate",
	}

	return &Participation{
		Features: []*Feature{
			NewFeature(
				participationClock,
				keys,
				&Class{
					Label:  "BroadLift",
					Within: participationPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"sentiment/advance_fraction",
						INCREASE,
						DECREASE,
					)},
				},
			),
			NewFeature(
				participationClock,
				keys,
				&Class{
					Label:  "LocalLeader",
					Within: participationPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"correlation/relative_return_energy",
						INCREASE,
						DECREASE,
					)},
				},
			),
			NewFeature(
				participationClock,
				keys,
				&Class{
					Label:  "FollowerMove",
					Within: participationPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"leadlag/best_lag_seconds",
						DECREASE,
						INCREASE,
					)},
				},
			),
			NewFeature(
				participationClock,
				keys,
				&Class{
					Label:  "IsolatedMove",
					Within: participationPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"correlation/relative_return_energy",
						EXPAND,
						DISSOLVE,
					)},
				},
			),
		},
	}
}
