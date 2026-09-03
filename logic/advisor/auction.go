package advisor

const (
	AuctionName              = "auction"
	auctionClock             = "pumpdump/completed_volume_bar_ordinal"
	auctionPredictionHorizon = uint64(1)
)

/*
Auction classifies the order book touch battle between aggressive market orders
and passive limit replenishment.
*/
type Auction struct {
	Features []*Feature
}

/*
NewAuction constructs the Auction advisor with competing features and
falsifiable predictions covering buyer/seller breakthrough and absorption.
*/
func NewAuction() *Auction {
	keys := []string{
		"cvd/signed_net_fraction",
		"cvd/net_notional_rate",
		"cvd/buy_notional_rate",
		"cvd/sell_notional_rate",
		"cvd/gross_notional",
		"cvd/net_notional",
		"cvd/gross_notional_rate_baseline",
		"cvd/gross_executed_quantity",
		"cvd/net_executed_quantity",
		"cvd/executed_quantity:buy",
		"cvd/executed_quantity:sell",
		"cvd/aggressive_notional:buy",
		"cvd/aggressive_notional:sell",
		"cvd/cumulative_notional_delta",
		"cvd/cumulative_volume_delta",
		"cvd/mean_trade_notional",
		"cvd/midpoint_log_return",
		"cvd/midpoint_return_rate",
		"cvd/midpoint_return_rate_baseline",
		"cvd/midpoint_return_rate_divergence",
		"cvd/signed_count_fraction",
		"cvd/trade_rate",
		"cvd/trade_count",
		"cvd/trade_count:buy",
		"cvd/trade_count:sell",
		"cvd/response_midpoint:at",
		"cvd/response_midpoint:from",
		"cvd/cvd_epoch_from",
		"cvd/midpoint_response_per_net_notional",
		"cvd/flow_aligned_midpoint_return",
		"cvd/signed_net_fraction_zscore",
		"hawkes/excitation_amplitude:buy_from_sell",
		"hawkes/excitation_amplitude:sell_from_buy",

		"toxicity/net_withdrawal_fraction:bid",
		"toxicity/net_withdrawal_fraction:ask",
		"toxicity/net_replenishment_fraction:bid",
		"toxicity/net_replenishment_fraction:ask",
		"toxicity/retreat_fraction:bid",
		"toxicity/retreat_fraction:ask",
		"toxicity/net_withdrawal_rate:bid",
		"toxicity/net_withdrawal_rate:ask",
		"toxicity/net_replenishment_rate:bid",
		"toxicity/net_replenishment_rate:ask",
		"toxicity/matched_touch_trade_quantity:ask",
		"toxicity/matched_touch_trade_quantity:bid",
		"toxicity/previous_touch_quantity:ask",
		"toxicity/previous_touch_quantity:bid",
		"depthflow/observed_notional_imbalance",
		"depthflow/observed_notional_rate",
		"depthflow/observed_notional_imbalance_zscore",
		"depthflow/add_notional:bid",
		"depthflow/add_notional:ask",
		"liquidity/touch_notional_imbalance",
		"liquidity/two_sided_touch_notional",
		"liquidity/relative_spread",
	}

	return &Auction{
		Features: []*Feature{
			NewFeature(
				auctionClock,
				keys,
				&Class{
					Label:  "BuyersBreakingThrough",
					Within: auctionPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"cvd/midpoint_response_per_net_notional",
						INCREASE,
						DECREASE,
					)},
				},
			),
			NewFeature(
				auctionClock,
				keys,
				&Class{
					Label:  "SellersAbsorbing",
					Within: auctionPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"cvd/flow_aligned_midpoint_return",
						DISSOLVE,
						EXPAND,
					)},
				},
			),
			NewFeature(
				auctionClock,
				keys,
				&Class{
					Label:  "SellersBreakingThrough",
					Within: auctionPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"cvd/midpoint_response_per_net_notional",
						INCREASE,
						DECREASE,
					)},
				},
			),
			NewFeature(
				auctionClock,
				keys,
				&Class{
					Label:  "BuyersAbsorbing",
					Within: auctionPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"cvd/flow_aligned_midpoint_return",
						DISSOLVE,
						EXPAND,
					)},
				},
			),
			NewFeature(
				auctionClock,
				keys,
				&Class{
					Label:  "Balanced",
					Within: auctionPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"cvd/signed_net_fraction",
						STAGNATE,
						EXPAND,
					)},
				},
			),
		},
	}
}
