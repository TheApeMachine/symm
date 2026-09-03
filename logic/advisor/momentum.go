package advisor

const (
	MomentumName  = "momentum"
	momentumClock = "pumpdump/completed_volume_bar_ordinal"
	// The first contract forecasts exactly the next completed causal volume bar.
	momentumPredictionHorizon = uint64(1)
)

/* Momentum declares the observation vectors for the momentum-state Advisor. */
type Momentum struct {
	Features []*Feature
}

/* NewMomentum defines the four competing momentum-state Features. */
func NewMomentum() *Momentum {
	return &Momentum{
		Features: []*Feature{
			NewFeature(
				momentumClock,
				[]string{
					// Direction, unusualness, and acceleration of price response.
					"pumpdump/midpoint_log_return",
					"pumpdump/midpoint_return_zscore",
					"pumpdump/midpoint_return_velocity",
					// Acceleration of completed economic throughput.
					"pumpdump/notional_rate_ratio",
					"pumpdump/notional_rate_zscore",
					"pumpdump/notional_rate_velocity",
					// Direction and acceleration of actual executed capital.
					"cvd/signed_net_fraction",
					"cvd/net_notional_rate_velocity",
					"cvd/flow_aligned_midpoint_return",
					// Scale-free event share and endogenous propagation by side.
					"hawkes/event_fraction:buy",
					"hawkes/event_fraction:sell",
					"hawkes/excitation_fraction:buy",
					"hawkes/excitation_fraction:sell",
					"hawkes/expected_descendants_from_buy",
					"hawkes/expected_descendants_from_sell",
					"hawkes/log_likelihood_gain_per_event_vs_poisson",
				},
				&Class{
					Label:  "Building",
					Within: momentumPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"pumpdump/notional_rate_velocity",
						INCREASE,
						DECREASE,
					)},
				},
			),
			NewFeature(
				momentumClock,
				[]string{
					// Continuing price response and whether its rate is holding.
					"pumpdump/midpoint_log_return",
					"pumpdump/midpoint_return_rate",
					"pumpdump/midpoint_return_velocity",
					// Continuing activity relative to its own causal baseline.
					"pumpdump/notional_rate_ratio",
					"pumpdump/notional_rate_velocity",
					// Persistent directed flow that still produces price response.
					"cvd/signed_net_fraction",
					"cvd/net_notional_rate",
					"cvd/flow_aligned_midpoint_return",
					// Persistence of the fitted arrival process.
					"hawkes/conditional_intensity",
					"hawkes/excitation_fraction:buy",
					"hawkes/excitation_fraction:sell",
					"hawkes/branching_spectral_radius",
					"hawkes/expected_descendants_from_buy",
					"hawkes/expected_descendants_from_sell",
					"hawkes/log_likelihood_gain_per_event_vs_poisson",
				},
				&Class{
					Label:  "Sustaining",
					Within: momentumPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"cvd/flow_aligned_midpoint_return",
						EXPAND,
						DISSOLVE,
					)},
				},
			),
			NewFeature(
				momentumClock,
				[]string{
					// Existing direction with weakening price response.
					"pumpdump/midpoint_log_return",
					"pumpdump/midpoint_return_velocity",
					// Declining economic participation.
					"pumpdump/notional_rate_velocity",
					// Flow is fading or no longer moving price efficiently.
					"cvd/signed_net_fraction",
					"cvd/signed_net_fraction_zscore",
					"cvd/net_notional_rate_velocity",
					"cvd/flow_aligned_midpoint_return",
					"cvd/midpoint_response_per_net_notional",
					// Arrival shortfall on the previously active side.
					"hawkes/standardized_innovation:buy",
					"hawkes/standardized_innovation:sell",
					"hawkes/excitation_fraction:buy",
					"hawkes/excitation_fraction:sell",
					"hawkes/log_likelihood_gain_per_event_vs_poisson",
				},
				&Class{
					Label:  "Stalling",
					Within: momentumPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"cvd/flow_aligned_midpoint_return",
						DISSOLVE,
						EXPAND,
					)},
				},
			),
			NewFeature(
				momentumClock,
				[]string{
					// Fast price response against its slower causal reference.
					"pumpdump/midpoint_log_return",
					"pumpdump/midpoint_return_velocity",
					"pumpdump/midpoint_return_baseline",
					"pumpdump/midpoint_return_divergence",
					"pumpdump/notional_rate_zscore",
					// Fast executed flow against its slower causal reference.
					"cvd/signed_net_fraction",
					"cvd/signed_net_fraction_zscore",
					"cvd/signed_net_fraction_baseline",
					"cvd/signed_net_fraction_divergence",
					"cvd/net_notional_rate_velocity",
					"cvd/flow_aligned_midpoint_return",
					// Scale-free arrival ownership and fitted propagation by side.
					"hawkes/event_fraction:buy",
					"hawkes/event_fraction:sell",
					"hawkes/excitation_fraction:buy",
					"hawkes/excitation_fraction:sell",
					"hawkes/expected_descendants_from_buy",
					"hawkes/expected_descendants_from_sell",
					"hawkes/log_likelihood_gain_per_event_vs_poisson",
				},
				&Class{
					Label:  "Reversing",
					Within: momentumPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"pumpdump/midpoint_return_divergence",
						EXPAND,
						DISSOLVE,
					)},
				},
			),
		},
	}
}
