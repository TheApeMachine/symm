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
	buyersBreakingThroughKeys := []string{
		"cvd/signed_net_fraction",
		"cvd/buy_notional_rate",
		"cvd/flow_aligned_midpoint_return",
		"cvd/midpoint_response_per_net_notional",
		"toxicity/retreat_fraction:ask",
	}

	sellersAbsorbingKeys := []string{
		"cvd/buy_notional_rate",
		"cvd/flow_aligned_midpoint_return",
		"toxicity/net_replenishment_fraction:ask",
		"toxicity/matched_touch_trade_quantity:ask",
		"hawkes/excitation_amplitude:sell_from_buy",
	}

	sellersBreakingThroughKeys := []string{
		"cvd/signed_net_fraction",
		"cvd/sell_notional_rate",
		"cvd/midpoint_response_per_net_notional",
		"toxicity/retreat_fraction:bid",
	}

	buyersAbsorbingKeys := []string{
		"cvd/sell_notional_rate",
		"cvd/flow_aligned_midpoint_return",
		"toxicity/net_replenishment_fraction:bid",
		"toxicity/matched_touch_trade_quantity:bid",
		"hawkes/excitation_amplitude:buy_from_sell",
	}

	balancedKeys := []string{
		"cvd/signed_net_fraction",
		"cvd/signed_net_fraction_mean",
		"liquidity/touch_notional_imbalance",
		"liquidity/relative_spread",
	}

	return &Auction{
		Features: []*Feature{
			NewFeature(
				auctionClock,
				buyersBreakingThroughKeys,
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
				sellersAbsorbingKeys,
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
				sellersBreakingThroughKeys,
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
				buyersAbsorbingKeys,
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
				balancedKeys,
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
