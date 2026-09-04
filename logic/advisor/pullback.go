package advisor

const (
	PullbackName              = "pullback"
	pullbackClock             = "pumpdump/completed_volume_bar_ordinal"
	pullbackPredictionHorizon = uint64(1)
)

/*
Pullback classifies counter-trend retracements, separating orderly breathing
from predatory liquidity sweeps and terminal structural breakdowns.
*/
type Pullback struct {
	Features []*Feature
}

/*
NewPullback constructs the Pullback advisor with competing features and
falsifiable predictions covering orderly pullbacks, liquidity sweeps,
structural breakdowns, and unresolved noise.
*/
func NewPullback() *Pullback {
	orderlyPullbackKeys := []string{
		"cvd/gross_notional_rate",
		"cvd/signed_net_fraction",
		"toxicity/retreat_fraction:bid",
		"hawkes/branching_spectral_radius",
		"morphology/morphology_change_zscore",
	}

	liquiditySweepKeys := []string{
		"toxicity/net_replenishment_fraction:bid",
		"toxicity/unfilled_residual_quantity:bid",
		"liquidity/touch_notional:bid",
		"cvd/net_notional_rate_velocity",
	}

	structuralBreakdownKeys := []string{
		"toxicity/net_withdrawal_fraction:bid",
		"toxicity/net_withdrawal_rate:bid",
		"pumpdump/spread_divergence",
		"liquidity/relative_spread",
		"morphology/morphology_change_zscore",
	}

	unresolvedKeys := []string{
		"hawkes/branching_spectral_radius",
		"liquidity/touch_notional_imbalance",
		"pumpdump/spread_divergence",
	}

	return &Pullback{
		Features: []*Feature{
			NewFeature(
				pullbackClock,
				orderlyPullbackKeys,
				&Class{
					Label:  "OrderlyPullback",
					Within: pullbackPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"cvd/gross_notional_rate",
						DECREASE,
						INCREASE,
					)},
				},
			),
			NewFeature(
				pullbackClock,
				liquiditySweepKeys,
				&Class{
					Label:  "LiquiditySweep",
					Within: pullbackPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"toxicity/net_replenishment_fraction:bid",
						INCREASE,
						DECREASE,
					)},
				},
			),
			NewFeature(
				pullbackClock,
				structuralBreakdownKeys,
				&Class{
					Label:  "StructuralBreakdown",
					Within: pullbackPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"toxicity/net_withdrawal_fraction:bid",
						EXPAND,
						DISSOLVE,
					)},
				},
			),
			NewFeature(
				pullbackClock,
				unresolvedKeys,
				&Class{
					Label:  "Unresolved",
					Within: pullbackPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"hawkes/branching_spectral_radius",
						STAGNATE,
						EXPAND,
					)},
				},
			),
		},
	}
}
