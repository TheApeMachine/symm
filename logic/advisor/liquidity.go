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
	wallBuildingKeys := []string{
		"liquidity/touch_notional:bid",
		"liquidity/two_sided_touch_notional",
		"liquidity/depth_ratio:bid",
		"liquidity/touch_quantity:bid",
	}

	vacuumFormingKeys := []string{
		"liquidity/relative_spread",
		"liquidity/spread_divergence",
		"liquidity/spread_zscore",
		"liquidity/relative_spread_baseline",
	}

	replenishingKeys := []string{
		"toxicity/net_replenished_quantity:bid",
		"toxicity/net_replenishment_rate:bid",
		"toxicity/net_replenishment_fraction:bid",
	}

	depletingKeys := []string{
		"toxicity/touch_fill_rate:bid",
		"toxicity/touch_fill_fraction:bid",
		"toxicity/touch_fill_quantity:bid",
	}

	balancedKeys := []string{
		"liquidity/touch_notional_imbalance",
		"liquidity/relative_spread",
		"liquidity/spread_ratio",
	}

	return &Liquidity{
		Features: []*Feature{
			NewFeature(
				liquidityClock,
				wallBuildingKeys,
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
				vacuumFormingKeys,
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
				replenishingKeys,
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
				depletingKeys,
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
				balancedKeys,
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
