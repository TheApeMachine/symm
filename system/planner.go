package system

import (
	"math"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/types"
)

/*
UninformativeDirectionConfidence is the probability boundary at which a
one-sided forecast provides no directional evidence. The regulator may raise
the confidence required to extend the adaptive forecast horizon above this
boundary; a lower value would call an unsupported direction informative.
*/
const UninformativeDirectionConfidence = 0.5

/*
PlannerConfig is the strategy policy configuration.

The Admission, MinimumConfidence, MinimumGraphScore, MinimumUtility, and
CausalAlpha fields are legacy semantic thresholds. They are retained for
configuration and telemetry compatibility only; they are inert in the live
decision path, which is governed by causal MCTS over economic outcomes. The
live fields are the search policy (SearchHorizon, MaxPositionUnits,
MCTSIterations, ExplorationConstant), the allocation policy
(MaxAllocationFraction), and the execution-cost policy (SlippageFraction).
*/
type PlannerConfig struct {
	// Admission is legacy semantic evidence admission; inert in the live path.
	Admission types.AdmissionPolicy
	// MaxAllocationFraction is the fraction of quote cash one entry may consume.
	MaxAllocationFraction float64
	// MinimumConfidence is legacy semantic admission; inert in the live path.
	MinimumConfidence float64
	// MinimumGraphScore is legacy semantic admission; inert in the live path.
	MinimumGraphScore float64
	// MinimumUtility is legacy semantic admission; inert in the live path.
	MinimumUtility float64
	// CausalAlpha is legacy evidence mixing; inert in the live path.
	CausalAlpha float64
	// MCTSIterations is the economic search iteration budget (strategy policy).
	MCTSIterations int
	// ExplorationConstant is the UCB exploration constant in reward units
	// (strategy policy; never disguised as market mathematics).
	ExplorationConstant float64
	// UncertaintyWeight scales the reward standard error in the UCB selection
	// rule. It is dimensionless strategy policy: the standard error it scales
	// already carries reward units, so the weighted term stays in reward
	// units like the mean and the C exploration term.
	UncertaintyWeight float64
	// SearchHorizon is the MCTS rollout horizon: the number of market
	// transitions evaluated per trajectory (strategy holding-horizon policy).
	SearchHorizon int
	// MaxPositionUnits caps the position size in units (exposure policy).
	// Scale is not executable by the broker yet, so the live path caps the
	// position at one sized unit and Scale is never offered by the search.
	MaxPositionUnits float64
	// SlippageFraction is modeled slippage per side as a fraction of notional
	// (strategy policy; stated explicitly, not disguised as market math).
	// It defaults to zero: the allocation already prices visible-depth impact
	// through the broker EntryCost at execution time, so zero means no
	// additional modeled slippage beyond the venue's observed impact. The
	// same value feeds both the economic search and the execution cost
	// assumption, so the two never drift apart.
	SlippageFraction float64
	// MinAdvantageFraction is the minimum economic advantage an entry must
	// clear over waiting before it may execute, as a fraction of the sized
	// entry notional. It is an explicit round-trip hurdle on top of the
	// fees and spread already charged inside the economic search: an entry
	// whose modeled edge cannot cover these costs plus this profit hurdle
	// is noise, not an opportunity. It is expressed as a fraction of the
	// allocatable notional so the hurdle scales with allocation, not with
	// absolute cash.
	MinAdvantageFraction float64
	// RelationInterval bounds how often per-symbol Relation estimates refresh
	// (infrastructure capacity, not a statistical horizon).
	RelationInterval time.Duration
	// MeasurementStep is the assumed measurement cadence used to declare the
	// causal schema's structural lags (explicit configuration).
	MeasurementStep time.Duration
	// RelationMaxLag is the explicit candidate lag search window for the
	// Relation plan (explicit configuration; bounded further by retained
	// history at estimation time).
	RelationMaxLag time.Duration
}

func NewPlannerConfig() *PlannerConfig {
	viper.SetDefault("trading.allocation.max_fraction", 0.1)
	viper.SetDefault("trading.admission.required_direction", 1.0)
	viper.SetDefault("trading.admission.minimum_thesis_score", 0.5)
	viper.SetDefault("trading.admission.minimum_confidence", 0.5)
	viper.SetDefault("trading.admission.minimum_support", 0.5)
	viper.SetDefault("trading.admission.maximum_contradiction", 0.3)
	viper.SetDefault(
		"trading.resonance.minimum_confidence",
		UninformativeDirectionConfidence,
	)
	viper.SetDefault("trading.evidence.minimum_score", -1.0)
	viper.SetDefault("trading.utility.minimum", 0.0)
	viper.SetDefault("trading.mcts.causal_alpha", 1.0)
	viper.SetDefault("trading.mcts.iterations", 16)
	viper.SetDefault("trading.mcts.exploration_constant", math.Sqrt2)
	viper.SetDefault("trading.mcts.uncertainty_weight", 0.5)
	viper.SetDefault("trading.mcts.search_horizon", 5)
	viper.SetDefault("trading.mcts.max_position_units", 1.0)
	viper.SetDefault("trading.mcts.slippage_fraction", 0.0)
	viper.SetDefault("trading.mcts.min_advantage_fraction", 0.0010)
	viper.SetDefault("trading.relation.interval_seconds", 1)
	viper.SetDefault("trading.relation.max_lag_seconds", 30)
	viper.SetDefault("trading.relation.measurement_step_seconds", 1)

	return &PlannerConfig{
		Admission: types.AdmissionPolicy{
			RequiredDirection:    viper.GetFloat64("trading.admission.required_direction"),
			MinimumThesisScore:   viper.GetFloat64("trading.admission.minimum_thesis_score"),
			MinimumConfidence:    viper.GetFloat64("trading.admission.minimum_confidence"),
			MinimumSupport:       viper.GetFloat64("trading.admission.minimum_support"),
			MaximumContradiction: viper.GetFloat64("trading.admission.maximum_contradiction"),
		},
		MaxAllocationFraction: viper.GetFloat64("trading.allocation.max_fraction"),
		MinimumConfidence:     viper.GetFloat64("trading.resonance.minimum_confidence"),
		MinimumGraphScore:     viper.GetFloat64("trading.evidence.minimum_score"),
		MinimumUtility:        viper.GetFloat64("trading.utility.minimum"),
		CausalAlpha:           viper.GetFloat64("trading.mcts.causal_alpha"),
		MCTSIterations:        viper.GetInt("trading.mcts.iterations"),
		ExplorationConstant:   viper.GetFloat64("trading.mcts.exploration_constant"),
		UncertaintyWeight:     viper.GetFloat64("trading.mcts.uncertainty_weight"),
		SearchHorizon:         viper.GetInt("trading.mcts.search_horizon"),
		MaxPositionUnits:      viper.GetFloat64("trading.mcts.max_position_units"),
		SlippageFraction:      viper.GetFloat64("trading.mcts.slippage_fraction"),
		MinAdvantageFraction:  viper.GetFloat64("trading.mcts.min_advantage_fraction"),
		RelationInterval:      time.Duration(viper.GetInt("trading.relation.interval_seconds")) * time.Second,
		MeasurementStep:       time.Duration(viper.GetInt("trading.relation.measurement_step_seconds")) * time.Second,
		RelationMaxLag:        time.Duration(viper.GetInt("trading.relation.max_lag_seconds")) * time.Second,
	}
}
