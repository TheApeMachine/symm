package perspectives

import (
	"fmt"

	"github.com/theapemachine/symm/numeric/adaptive"
)

type CategoryType string

const (
	CategoryTypeNone           CategoryType = ""
	CategoryLaminar            CategoryType = "laminar"
	CategoryTurbulent          CategoryType = "turbulent"
	CategoryInertial           CategoryType = "inertial"
	CategoryViscous            CategoryType = "viscous"
	CategoryFrenzy             CategoryType = "frenzy"
	CategorySaturation         CategoryType = "saturation"
	CategoryOrganic            CategoryType = "organic"
	CategoryExhaustion         CategoryType = "exhaustion"
	CategoryHiddenAbsorption   CategoryType = "hidden_absorption"
	CategoryAggressiveDrive    CategoryType = "aggressive_drive"
	CategoryStochasticBalance  CategoryType = "stochastic_balance"
	CategoryVolumeStarvation   CategoryType = "volume_starvation"
	CategoryLoadedImbalance    CategoryType = "loaded_imbalance"
	CategorySpoofTrap          CategoryType = "spoof_trap"
	CategoryBookThinning       CategoryType = "book_thinning"
	CategoryDenseNeutrality    CategoryType = "dense_neutrality"
	CategoryInefficientLag     CategoryType = "inefficient_lag"
	CategorySynchronizedDrift  CategoryType = "synchronized_drift"
	CategoryDecoupledMove      CategoryType = "decoupled_move"
	CategoryAnchorStall        CategoryType = "anchor_stall"
	CategoryVerticalIgnition   CategoryType = "vertical_ignition"
	CategoryCoiledCompression  CategoryType = "coiled_compression"
	CategoryOrganicTrend       CategoryType = "organic_trend"
	CategoryFadedExhaustion    CategoryType = "faded_exhaustion"
	CategoryExtremeScarcity    CategoryType = "extreme_scarcity"
	CategoryMedianDepth        CategoryType = "median_depth"
	CategoryRobustLiquidity    CategoryType = "robust_liquidity"
	CategoryRiskOnSurge        CategoryType = "risk_on_surge"
	CategoryDivergentMove      CategoryType = "divergent_move"
	CategorySystemicSlump      CategoryType = "systemic_slump"
	CategoryLiquidityVacuum    CategoryType = "liquidity_vacuum"
	CategoryToxicBluff         CategoryType = "toxic_bluff"
	CategoryHardSupport        CategoryType = "hard_support"
	CategorySystemicHerd       CategoryType = "systemic_herd"
	CategoryDecoupledAlpha     CategoryType = "decoupled_alpha"
	CategoryStochasticNoise    CategoryType = "stochastic_noise"
	CategoryDivergentStress    CategoryType = "divergent_stress"
	CategoryEndogenousAlpha    CategoryType = "endogenous_alpha"
	CategorySystemicBeta       CategoryType = "systemic_beta"
	CategoryLiquidityShock     CategoryType = "liquidity_shock"
	CategoryCausalNoise        CategoryType = "causal_noise"
	CategoryMechanicalCollapse CategoryType = "mechanical_collapse"
	CategoryThermalExhaustion  CategoryType = "thermal_exhaustion"
	CategoryFragileExpansion   CategoryType = "fragile_expansion"
	CategoryActiveReversal     CategoryType = "active_reversal"
)

/*
Categories is a threshold-band classifier on one scalar observation. It does not
score every class and pick the highest — it finds which band the observation falls
into (see adaptive.Classifier).
*/
type Categories adaptive.Classifier

/*
NewCategories builds a band classifier. upper holds len(categories)-1 observation
thresholds; categories lists the bands low-to-high.
*/
func NewCategories(upper []float64, categories []CategoryType) (Categories, error) {
	if len(categories) == 0 {
		return Categories{}, fmt.Errorf("perspectives: NewCategories needs at least one category")
	}

	if len(upper) != len(categories)-1 {
		return Categories{}, fmt.Errorf(
			"perspectives: NewCategories needs len(upper) == len(categories)-1, got %d and %d",
			len(upper), len(categories),
		)
	}

	codes := make([]float64, len(categories))
	labels := make([]string, len(categories))

	for index, category := range categories {
		codes[index] = float64(index)
		labels[index] = string(category)
	}

	classifier := adaptive.NewClassifier(upper, codes, labels)

	return Categories(*classifier), nil
}

func (categories Categories) classifier() *adaptive.Classifier {
	return (*adaptive.Classifier)(&categories)
}

/*
Classify maps one observation into the band it falls in.
*/
func (categories Categories) Classify(observation float64) (CategoryType, error) {
	code, err := categories.classifier().Code(observation)

	if err != nil {
		return CategoryTypeNone, err
	}

	return CategoryType(categories.classifier().Label(code)), nil
}

/*
Clarity is band margin for the observation — how deep inside its band, not shift
confidence over time.
*/
func (categories Categories) Clarity(observation float64) float64 {
	return categories.classifier().Confidence(observation)
}

/*
Category tracks the current assignment and accumulated shift evidence.
*/
type Category struct {
	Type       CategoryType
	Confidence *adaptive.Accumulator
}

func NewCategory(categoryType CategoryType) *Category {
	return &Category{
		Type:       categoryType,
		Confidence: adaptive.NewAccumulator(0),
	}
}

/*
Observe updates Type and charges the accumulator when the category shifts.
evidence is typically Clarity or strength at the moment of change.
*/
func (category *Category) Observe(next CategoryType, evidence float64) (float64, error) {
	if category == nil || category.Confidence == nil {
		return 0, fmt.Errorf("perspectives: Category.Observe nil receiver")
	}

	signal := 0.0

	if next != category.Type {
		category.Type = next
		signal = evidence
	}

	return category.Confidence.Next(0, signal)
}
