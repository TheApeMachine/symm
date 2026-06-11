package logic

import (
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
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

type Category struct {
	Type          CategoryType `yaml:"type"`
	Confidence    float64      `yaml:"confidence"`
	Surprise      float64      `yaml:"surprise"`
	confidenceRef string       `yaml:"-"`
	surpriseRef   string       `yaml:"-"`
}

func NewCategory(
	categoryType CategoryType, confidence float64, surprise float64,
) *Category {
	return &Category{
		Type: categoryType, Confidence: confidence, Surprise: surprise,
	}
}

func (category *Category) UnmarshalYAML(node *yaml.Node) error {
	type categoryFields struct {
		Type       CategoryType `yaml:"type"`
		Confidence yaml.Node    `yaml:"confidence"`
		Surprise   yaml.Node    `yaml:"surprise"`
	}

	fields := categoryFields{}

	if err := node.Decode(&fields); err != nil {
		return err
	}

	category.Type = fields.Type

	if err := category.decodeConfidence(fields.Confidence); err != nil {
		return err
	}

	return category.decodeSurprise(fields.Surprise)
}

func (category *Category) decodeConfidence(node yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		raw := strings.TrimSpace(node.Value)

		if after, ok := strings.CutPrefix(raw, "$"); ok {
			category.confidenceRef = after
			category.Confidence = 0

			return nil
		}
	}

	if err := node.Decode(&category.Confidence); err != nil {
		return fmt.Errorf("logic: category confidence: %w", err)
	}

	return nil
}

func (category *Category) decodeSurprise(node yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		raw := strings.TrimSpace(node.Value)

		if after, ok := strings.CutPrefix(raw, "$"); ok {
			category.surpriseRef = after
			category.Surprise = 0

			return nil
		}
	}

	if node.IsZero() {
		return nil
	}

	if err := node.Decode(&category.Surprise); err != nil {
		return fmt.Errorf("logic: category surprise: %w", err)
	}

	return nil
}
