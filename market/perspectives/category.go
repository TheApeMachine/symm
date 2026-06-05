package perspectives

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
adaptive.Classifier.Standout.
*/
func (categories Categories) Standout(observation float64) float64 {
	return categories.classifier().Standout(observation)
}

/*
ScoreCategorySNR folds category standout into the symbol's running noise floor
and returns sigma above that baseline. Standout must be a unit band margin in
[0, 1]; invalid input or an immeasurable floor returns an error.
*/
func ScoreCategorySNR(floor *adaptive.SNRField, symbol string, standout float64) (float64, error) {
	if floor == nil {
		return 0, errnie.Error(errors.New("perspectives: ScoreCategorySNR nil floor"))
	}

	if symbol == "" {
		return 0, errnie.Error(errors.New("perspectives: ScoreCategorySNR empty symbol"))
	}

	if err := validateUnitMargin("standout", standout); err != nil {
		return 0, errnie.Error(err)
	}

	return floor.Score(symbol, standout)
}

/*
AssignCategorySNR scores standout and writes the result onto measurement.SNR.
*/
func AssignCategorySNR(
	measurement *Measurement,
	floor *adaptive.SNRField,
	standout float64,
) error {
	snr, err := ScoreCategorySNR(floor, measurement.Symbol, standout)

	if err != nil {
		return errnie.Error(err)
	}

	measurement.SNR = snr

	return nil
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

// confidenceFloor keeps emitted confidence inside the open interval (floor,
// 1-floor). A reading right on a band boundary is maximally uncertain, not "zero
// confidence", and one deep in a band is near-certain, not a saturated 1 — exact 0
// or 1 are clamping artifacts of a misbehaving signal, never a genuine
// category-selection certainty. Every signal routes its confidence through Observe,
// so clamping here covers them all (it is a no-op for the classifier-based signals,
// whose clarity is already inside the interval).
const confidenceFloor = 0.02

func clampConfidence(clarity float64) float64 {
	if clarity < confidenceFloor {
		return confidenceFloor
	}

	if clarity > 1-confidenceFloor {
		return 1 - confidenceFloor
	}

	return clarity
}

/*
Observe updates Type and charges the accumulator when the category shifts.
It returns clarity — instantaneous band margin for this reading, clamped away from
exactly 0/1. The accumulator is charged with standout (winner margin over
alternatives), not clarity.
*/
func (category *Category) Observe(
	next CategoryType,
	clarity float64,
	standout float64,
) (float64, error) {
	if category == nil || category.Confidence == nil {
		return 0, errnie.Error(errors.New("perspectives: Category.Observe nil receiver"))
	}

	if err := validateUnitMargin("clarity", clarity); err != nil {
		return 0, errnie.Error(err)
	}

	if err := validateUnitMargin("standout", standout); err != nil {
		return 0, errnie.Error(err)
	}

	if next != category.Type {
		category.Type = next

		if _, err := category.Confidence.Next(0, standout); err != nil {
			return 0, errnie.Error(err)
		}
	}

	return clampConfidence(clarity), nil
}

func validateUnitMargin(name string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return errnie.Error(fmt.Errorf("perspectives: Category.Observe invalid %s", name))
	}

	if value > 1 {
		return errnie.Error(fmt.Errorf("perspectives: Category.Observe %s above unit band: %v", name, value))
	}

	return nil
}
