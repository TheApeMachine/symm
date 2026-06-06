package types

import (
	"errors"
	"fmt"
	"math"

	"github.com/theapemachine/errnie"

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
		return Categories{}, errnie.Error(errors.New("perspectives: NewCategories needs at least one category"))
	}

	if len(upper) != len(categories)-1 {
		return Categories{}, errnie.Error(fmt.Errorf(
			"perspectives: NewCategories needs len(upper) == len(categories)-1, got %d and %d",
			len(upper), len(categories),
		))
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
		return CategoryTypeNone, errnie.Error(err)
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
Standout is winner margin over the nearest competing category band — see
adaptive.Classifier.Standout. It measures spatial band clarity, not temporal
surprise; SNR is scored separately via CategorySurpriseField.
*/
func (categories Categories) Standout(observation float64) float64 {
	return categories.classifier().Standout(observation)
}

/*
Category carries the signal's current category selection. Type is the last
category the signal chose; signals that condition on their previous pick (see
signal/hawkes) read it. Confidence is not stored here — it is per-reading, produced
by the signal and carried on the Measurement, never accumulated across readings.
*/
type Category struct {
	Type CategoryType
}

func NewCategory(categoryType CategoryType) *Category {
	return &Category{Type: categoryType}
}

/*
Observe records the signal's category selection and validates the confidence it
reports in that selection. Confidence is the signal's finite unit-band evidence
for a selection; zero is not publishable evidence, and values above one indicate
invalid signal math. SNR (temporal surprise from categorical Shannon surprisal) is
scored separately.
*/
func (category *Category) Observe(next CategoryType, confidence float64) error {
	if category == nil {
		return errnie.Error(errors.New("perspectives: Category.Observe nil receiver"))
	}

	if err := validateUnitMargin("confidence", confidence); err != nil {
		return err
	}

	category.Type = next

	return nil
}

func validateUnitMargin(name string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return errnie.Error(fmt.Errorf("perspectives: invalid %s: %v", name, value))
	}

	if value > 1 {
		return errnie.Error(fmt.Errorf("perspectives: %s above unit band: %v", name, value))
	}

	return nil
}
