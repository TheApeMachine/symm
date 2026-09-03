package advisor

const (
	ProfitRunName              = "profit_run"
	profitRunClock             = "pumpdump/completed_volume_bar_ordinal"
	profitRunPredictionHorizon = uint64(1)
)

/*
ProfitRun classifies the holding dynamics of an active trade, distinguishing
extending runners and healthy consolidation from momentum exhaustion and
unacceptable give-back.
*/
type ProfitRun struct {
	Features []*Feature
}

/*
NewProfitRun constructs the ProfitRun advisor with competing features and
falsifiable predictions covering extending runs, consolidation, exhaustion,
and giving back.
*/
func NewProfitRun() *ProfitRun {
	keys := []string{
		"pumpdump/midpoint_log_return",
		"pumpdump/midpoint_return_velocity",
		"pumpdump/midpoint_return_zscore",
		"pumpdump/negative_midpoint_return",
		"pumpdump/positive_midpoint_return",
		"pumpdump/notional_rate",
		"pumpdump/notional_rate_baseline",
		"pumpdump/notional_rate_ratio",
		"pumpdump/notional_rate_velocity",
		"pumpdump/spread_divergence_velocity",
		"pumpdump/notional_rate_divergence",
		"pumpdump/relative_spread_baseline",
		"pumpdump/spread",
		"pumpdump/spread_ratio",
		"pumpdump/trade_notional",
		"pumpdump/trade_price",
		"pumpdump/trade_quantity",
		"pumpdump/trade_rate",
		"pumpdump/volume_bar_duration",
		"pumpdump/volume_bar_notional",
		"pumpdump/volume_bar_target_quantity",
		"pumpdump/volume_bar_trade_count",
		"pumpdump/midpoint",
		"pumpdump/midpoint:at",
		"pumpdump/midpoint:from",
		"pumpdump/best_bid",
		"pumpdump/best_ask",
		"cvd/signed_net_fraction",
		"cvd/flow_aligned_midpoint_return",
		"cvd/midpoint_response_per_net_notional",
		"cvd/net_notional_rate",
		"cvd/mean_trade_notional",
		"hawkes/arrival_rate:buy",
		"hawkes/arrival_rate:sell",
		"hawkes/background_rate",
		"hawkes/compensator:buy",
		"hawkes/compensator:sell",
		"hawkes/event_count",
		"hawkes/event_count:buy",
		"hawkes/event_count:sell",
		"hawkes/branching_spectral_radius",
		"hawkes/spectral_radius_velocity",
		"hawkes/excitation_fraction:buy",
		"hawkes/excitation_amplitude:buy_from_buy",
		"hawkes/excitation_amplitude:sell_from_sell",
		"hawkes/excitation_decay:buy_from_buy",
		"hawkes/excitation_decay:buy_from_sell",
		"hawkes/excitation_decay:sell_from_buy",
		"hawkes/excitation_decay:sell_from_sell",
		"hawkes/excitation_timescale:buy_from_buy",
		"hawkes/excitation_timescale:buy_from_sell",
		"hawkes/excitation_timescale:sell_from_buy",
		"hawkes/excitation_timescale:sell_from_sell",
		"hawkes/log_likelihood:hawkes",
		"hawkes/log_likelihood:poisson",
		"hawkes/log_likelihood:self_only",
		"hawkes/log_likelihood_gain_per_event_vs_self_only",
		"hawkes/log_likelihood_gain_vs_poisson",
		"hawkes/log_likelihood_gain_vs_self_only",
		"hawkes/log_likelihood_per_event:hawkes",
		"hawkes/offspring:buy_from_buy",
		"hawkes/offspring:buy_from_sell",
		"hawkes/offspring:sell_from_buy",
		"hawkes/offspring:sell_from_sell",
		"hawkes/conditional_intensity",
		"hawkes/conditional_intensity:buy",
		"hawkes/standardized_innovation:buy",
		"hawkes/count_innovation:buy",
		"hawkes/snr",
		"derivatives/open_interest_growth_rate",
		"derivatives/open_interest_growth_velocity",
		"derivatives/open_interest_growth_zscore",
		"derivatives/basis_velocity",
		"derivatives/basis_zscore",
		"toxicity/net_withdrawal_fraction:bid",
		"toxicity/net_withdrawal_fraction:ask",
		"toxicity/retreat_fraction:bid",
		"toxicity/retreat_fraction:ask",
	}

	return &ProfitRun{
		Features: []*Feature{
			NewFeature(
				profitRunClock,
				keys,
				&Class{
					Label:  "Extending",
					Within: profitRunPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"pumpdump/midpoint_return_velocity",
						INCREASE,
						DECREASE,
					)},
				},
			),
			NewFeature(
				profitRunClock,
				keys,
				&Class{
					Label:  "Consolidating",
					Within: profitRunPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"pumpdump/midpoint_log_return",
						STAGNATE,
						EXPAND,
					)},
				},
			),
			NewFeature(
				profitRunClock,
				keys,
				&Class{
					Label:  "Exhausting",
					Within: profitRunPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"cvd/flow_aligned_midpoint_return",
						DISSOLVE,
						EXPAND,
					)},
				},
			),
			NewFeature(
				profitRunClock,
				keys,
				&Class{
					Label:  "GivingBack",
					Within: profitRunPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"pumpdump/midpoint_return_velocity",
						DECREASE,
						INCREASE,
					)},
				},
			),
		},
	}
}
