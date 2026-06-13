package logic

/*
ExitTier classifies exit signals into thesis invalidation, risk deterioration, or profit exhaustion.
*/
type ExitTier int

const (
	ExitTierThesisInvalidation ExitTier = iota + 1
	ExitTierRiskDeterioration
	ExitTierProfitExhaustion
)

/*
ExitTierForCategory maps exit-oriented categories to partial or full liquidation tiers.
*/
func ExitTierForCategory(category CategoryType) ExitTier {
	switch category {
	case CategoryMechanicalCollapse,
		CategoryActiveReversal,
		CategoryLiquidityShock,
		CategoryTurbulent:
		return ExitTierThesisInvalidation
	case CategorySaturation,
		CategoryThermalExhaustion,
		CategoryFragileExpansion:
		return ExitTierRiskDeterioration
	default:
		return ExitTierProfitExhaustion
	}
}

/*
ExitFractionForTier returns the target liquidation fraction for an exit tier.
*/
func ExitFractionForTier(tier ExitTier, requestedFraction float64) float64 {
	switch tier {
	case ExitTierThesisInvalidation:
		return 1
	case ExitTierRiskDeterioration:
		if requestedFraction > 0 && requestedFraction < 1 {
			return requestedFraction
		}

		return 0.5
	case ExitTierProfitExhaustion:
		if requestedFraction > 0 && requestedFraction < 1 {
			return requestedFraction
		}

		return 0.33
	default:
		return 1
	}
}
