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
	keys := []string{
		"cvd/gross_notional_rate",
		"cvd/signed_net_fraction",
		"cvd/net_notional_rate_velocity",
		"toxicity/net_withdrawal_fraction:bid",
		"toxicity/retreat_fraction:bid",
		"toxicity/net_replenishment_fraction:bid",
		"toxicity/net_withdrawal_rate:bid",
		"toxicity/net_replenishment_rate:bid",
		"toxicity/withdrawal_fraction_velocity:bid",
		"toxicity/unfilled_residual_quantity:bid",
		"toxicity/previous_best_price:bid",
		"depthflow/observed_notional_imbalance",
		"depthflow/observed_notional_rate",
		"depthflow/observed_notional_imbalance_zscore",
		"depthflow/add_notional:bid",
		"hawkes/excitation_fraction:sell",
		"hawkes/branching_spectral_radius",
		"hawkes/standardized_innovation:sell",
		"hawkes/event_fraction:sell",
		"hawkes/expected_descendants_from_sell",
		"hawkes/background_rate:sell",
		"hawkes/count_innovation:sell",
		"derivatives/liquidation_notional:sell",
		"liquidity/touch_notional:bid",
		"liquidity/touch_notional_imbalance",
		"liquidity/relative_spread",
		"pumpdump/spread_divergence",
		"pumpdump/spread_divergence_velocity",
		"morphology/book_shape_distance",
		"morphology/book_shape_ks",
		"morphology/morphology_change",
	}

	return &Pullback{
		Features: []*Feature{
			NewFeature(
				pullbackClock,
				keys,
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
				keys,
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
				keys,
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
				keys,
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
