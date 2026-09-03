package advisor

const (
	BasisName              = "basis"
	basisClock             = "pumpdump/completed_volume_bar_ordinal"
	basisPredictionHorizon = uint64(1)
)

/*
Basis classifies perpetual futures basis dynamics, leverage positioning,
liquidation cascades, and derivative-spot lead/lag.
*/
type Basis struct {
	Features []*Feature
}

/*
NewBasis constructs the Basis advisor with competing features and
falsifiable predictions covering leverage squeeze, premium expansion,
discount expansion, liquidation cascade, and neutral basis states.
*/
func NewBasis() *Basis {
	keys := []string{
		"derivatives/basis",
		"derivatives/basis_velocity",
		"derivatives/basis_change",
		"derivatives/basis_baseline",
		"derivatives/derivative_spot_log_basis",
		"derivatives/derivative_index_log_basis",
		"derivatives/index_spot_log_basis",
		"derivatives/gross_derivative_trade_notional",

		"derivatives/gross_liquidation_notional",
		"derivatives/liquidation_notional:buy",
		"derivatives/liquidation_notional:sell",
		"derivatives/liquidation_share_velocity",
		"derivatives/liquidation_signed_fraction",
		"derivatives/open_interest",
		"derivatives/open_interest_change",
		"derivatives/open_interest_log_change",
		"derivatives/open_interest_growth_baseline",
		"derivatives/log_basis",
		"derivatives/basis_closure_error",
		"derivatives/derivative_price",
		"derivatives/reference_price",
		"derivatives/derivative_log_return",
		"derivatives/reference_log_return",
		"derivatives/return_gap",
		"derivatives/return_gap_velocity",
		"derivatives/return_gap_zscore",

		"hawkes/arrival_rate",
		"hawkes/conditional_intensity:buy",
		"hawkes/conditional_intensity:sell",
		"hawkes/background_rate:buy",
		"hawkes/background_rate:sell",
		"hawkes/count_innovation:buy",
		"hawkes/count_innovation:sell",
		"hawkes/spectral_radius_velocity",
		"hawkes/branching_spectral_radius",

		"cvd/gross_notional_rate_ratio",
		"cvd/gross_notional_rate_velocity",
		"cvd/gross_notional_rate_zscore",
		"cvd/mean_trade_notional",
	}

	return &Basis{
		Features: []*Feature{
			NewFeature(
				basisClock,
				keys,
				&Class{
					Label:  "LeverageSqueeze",
					Within: basisPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"derivatives/basis_velocity",
						INCREASE,
						DECREASE,
					)},
				},
			),
			NewFeature(
				basisClock,
				keys,
				&Class{
					Label:  "PremiumExpanding",
					Within: basisPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"derivatives/derivative_spot_log_basis",
						EXPAND,
						DISSOLVE,
					)},
				},
			),
			NewFeature(
				basisClock,
				keys,
				&Class{
					Label:  "DiscountExpanding",
					Within: basisPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"derivatives/derivative_spot_log_basis",
						DISSOLVE,
						EXPAND,
					)},
				},
			),
			NewFeature(
				basisClock,
				keys,
				&Class{
					Label:  "LiquidationsCascading",
					Within: basisPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"derivatives/gross_liquidation_notional",
						INCREASE,
						DECREASE,
					)},
				},
			),
			NewFeature(
				basisClock,
				keys,
				&Class{
					Label:  "NeutralBasis",
					Within: basisPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"derivatives/basis_change",
						STAGNATE,
						EXPAND,
					)},
				},
			),
		},
	}
}
