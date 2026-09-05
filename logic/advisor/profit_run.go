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
	extendingKeys := []string{
		"pumpdump/midpoint_return_velocity",
		"pumpdump/notional_rate_velocity",
		"pumpdump/positive_midpoint_return",
		"cvd/signed_net_fraction",
	}

	consolidatingKeys := []string{
		"pumpdump/midpoint_log_return",
		"pumpdump/notional_rate",
		"pumpdump/spread_ratio",
		"pumpdump/trade_rate",
	}

	exhaustingKeys := []string{
		"cvd/flow_aligned_midpoint_return",
		"cvd/midpoint_response_per_net_notional",
		"toxicity/retreat_fraction:ask",
		"pumpdump/midpoint_return_zscore",
		"pumpdump/notional_rate_velocity",
	}

	givingBackKeys := []string{
		"pumpdump/midpoint_return_velocity",
		"pumpdump/negative_midpoint_return",
		"toxicity/net_withdrawal_fraction:bid",
		"cvd/net_notional_rate",
		"cvd/signed_net_fraction",
	}

	return &ProfitRun{
		Features: []*Feature{
			NewFeature(
				profitRunClock,
				extendingKeys,
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
				consolidatingKeys,
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
				exhaustingKeys,
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
				givingBackKeys,
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
