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
	leverageSqueezeKeys := []string{
		"derivatives/basis_velocity",
		"derivatives/basis",
		"derivatives/open_interest",
		"derivatives/gross_liquidation_notional",
	}

	premiumExpandingKeys := []string{
		"derivatives/derivative_spot_log_basis",
		"derivatives/log_basis",
		"derivatives/derivative_log_return",
		"derivatives/return_gap",
	}

	discountExpandingKeys := []string{
		"derivatives/derivative_spot_log_basis",
		"derivatives/index_spot_log_basis",
		"derivatives/reference_log_return",
		"derivatives/return_gap_velocity",
	}

	liquidationsCascadingKeys := []string{
		"derivatives/gross_liquidation_notional",
		"derivatives/liquidation_notional:buy",
		"derivatives/liquidation_notional:sell",
		"derivatives/liquidation_signed_fraction",
		"derivatives/gross_derivative_trade_notional",
	}

	neutralBasisKeys := []string{
		"derivatives/basis_change",
		"derivatives/basis_baseline",
		"derivatives/derivative_index_log_basis",
		"derivatives/spot_price",
	}

	return &Basis{
		Features: []*Feature{
			NewFeature(
				basisClock,
				leverageSqueezeKeys,
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
				premiumExpandingKeys,
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
				discountExpandingKeys,
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
				liquidationsCascadingKeys,
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
				neutralBasisKeys,
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
