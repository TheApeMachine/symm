package advisor

const (
	LiquidityName              = "liquidity"
	liquidityClock             = "pumpdump/completed_volume_bar_ordinal"
	liquidityPredictionHorizon = uint64(1)
)

/*
Liquidity classifies order book microstructure, touch resilience,
depth depletion, and geometric book profile.
*/
type Liquidity struct {
	Features []*Feature
}

/*
NewLiquidity constructs the Liquidity advisor with competing features and
falsifiable predictions covering wall building, vacuum formation, replenishing,
depleting, and balanced order book states.
*/
func NewLiquidity() *Liquidity {
	keys := []string{
		"liquidity/touch_notional:bid",
		"liquidity/touch_notional:ask",
		"liquidity/touch_notional_baseline:bid",
		"liquidity/touch_notional_baseline:ask",
		"liquidity/touch_quantity:bid",
		"liquidity/touch_quantity:ask",
		"liquidity/touch_notional_imbalance",
		"liquidity/two_sided_touch_notional",
		"liquidity/relative_spread",
		"liquidity/relative_spread_baseline",
		"liquidity/spread",
		"liquidity/spread_ratio",
		"liquidity/spread_divergence",
		"liquidity/spread_divergence_velocity",
		"liquidity/spread_noise_scale",
		"liquidity/spread_zscore",
		"liquidity/depth_ratio:bid",
		"liquidity/depth_ratio:ask",
		"liquidity/depth_divergence:bid",
		"liquidity/depth_divergence:ask",
		"liquidity/depth_noise_scale:bid",
		"liquidity/depth_noise_scale:ask",
		"liquidity/depth_zscore:bid",
		"liquidity/depth_zscore:ask",
		"liquidity/divergence_velocity:bid",
		"liquidity/divergence_velocity:ask",
		"liquidity/divergence_velocity_snr:bid",
		"liquidity/divergence_velocity_snr:ask",
		"liquidity/spread_divergence_velocity_snr",
		"liquidity/best_bid_price",
		"liquidity/best_ask_price",
		"liquidity/midpoint",

		"toxicity/touch_fill_fraction:ask",
		"toxicity/touch_fill_fraction:bid",
		"toxicity/touch_fill_rate:ask",
		"toxicity/touch_fill_rate:bid",
		"toxicity/touch_quantity:ask",
		"toxicity/touch_quantity:bid",
		"toxicity/touch_fill_quantity:ask",
		"toxicity/touch_fill_quantity:bid",
		"toxicity/touch_price_log_change:ask",
		"toxicity/touch_price_log_change:bid",
		"toxicity/net_replenished_quantity:ask",
		"toxicity/net_replenished_quantity:bid",
		"toxicity/net_withdrawn_quantity:ask",
		"toxicity/net_withdrawn_quantity:bid",
		"toxicity/retreat_rate:ask",
		"toxicity/retreat_rate:bid",
		"toxicity/retreated_quantity:ask",
		"toxicity/retreated_quantity:bid",
		"toxicity/retreat_fraction_baseline:ask",
		"toxicity/retreat_fraction_baseline:bid",
		"toxicity/retreat_fraction_zscore:ask",
		"toxicity/retreat_fraction_zscore:bid",
		"toxicity/fill_fraction_velocity:ask",
		"toxicity/fill_fraction_velocity:bid",
		"toxicity/fill_fraction_baseline:ask",
		"toxicity/fill_fraction_baseline:bid",
		"toxicity/fill_fraction_divergence:ask",
		"toxicity/fill_fraction_divergence:bid",
		"toxicity/withdrawal_fraction_velocity:ask",
		"toxicity/withdrawal_fraction_velocity:bid",
		"toxicity/withdrawal_fraction_baseline:ask",
		"toxicity/withdrawal_fraction_baseline:bid",
		"toxicity/withdrawal_fraction_divergence:ask",
		"toxicity/withdrawal_fraction_divergence:bid",
		"toxicity/withdrawal_fraction_zscore:ask",
		"toxicity/withdrawal_fraction_zscore:bid",
		"toxicity/unfilled_residual_quantity:ask",
		"toxicity/previous_best_price:ask",
		"toxicity/best_price:ask",
		"toxicity/best_price:bid",
		"toxicity/bracket_trade_quantity",

		"depthflow/observed_notional",
		"depthflow/observed_notional_imbalance_baseline",
		"depthflow/observed_notional_rate_baseline",
		"depthflow/observed_notional_rate_divergence",
		"depthflow/delete_count:bid",
		"depthflow/delete_count:ask",
		"depthflow/mutation_count:bid",
		"depthflow/mutation_count:ask",
		"depthflow/modify_remaining_notional:bid",
		"depthflow/modify_remaining_notional:ask",
		"depthflow/observed_notional:bid",
		"depthflow/observed_notional:ask",
		"depthflow/mutation_activity_imbalance",

		"morphology/concentration:ask",
		"morphology/concentration:bid",
		"morphology/entropy:ask",
		"morphology/entropy:bid",
		"morphology/book_shape_distance",
		"morphology/book_shape_ks",
		"morphology/morphology_change",
		"morphology/morphology_change_baseline",
		"morphology/morphology_change_zscore",
	}

	return &Liquidity{
		Features: []*Feature{
			NewFeature(
				liquidityClock,
				keys,
				&Class{
					Label:  "WallBuilding",
					Within: liquidityPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"liquidity/two_sided_touch_notional",
						INCREASE,
						DECREASE,
					)},
				},
			),
			NewFeature(
				liquidityClock,
				keys,
				&Class{
					Label:  "VacuumForming",
					Within: liquidityPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"liquidity/relative_spread",
						EXPAND,
						DISSOLVE,
					)},
				},
			),
			NewFeature(
				liquidityClock,
				keys,
				&Class{
					Label:  "Replenishing",
					Within: liquidityPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"toxicity/net_replenished_quantity:bid",
						INCREASE,
						DECREASE,
					)},
				},
			),
			NewFeature(
				liquidityClock,
				keys,
				&Class{
					Label:  "Depleting",
					Within: liquidityPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"toxicity/touch_fill_rate:bid",
						INCREASE,
						DECREASE,
					)},
				},
			),
			NewFeature(
				liquidityClock,
				keys,
				&Class{
					Label:  "Balanced",
					Within: liquidityPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"liquidity/touch_notional_imbalance",
						STAGNATE,
						EXPAND,
					)},
				},
			),
		},
	}
}
