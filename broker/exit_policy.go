package broker

import (
	"fmt"
	"math"
)

// AdaptiveExitConfig defines the starting adaptive policy. Thresholds are
// expressed in net PnL percentage points, not fractions.
type AdaptiveExitConfig struct {
	// HardLossLimitPct is the absolute maximum loss allowed for a position.
	// Zero disables this cap; production wiring derives it from risk config.
	HardLossLimitPct float64

	// LossFrictionMultiple is the minimum risk floor in round-trip friction units.
	LossFrictionMultiple float64
	// LossStepMultiple scales observed positive PnL step size into a risk floor.
	LossStepMultiple float64
	// TrailFrictionMultiple is the minimum trailing distance in friction units.
	TrailFrictionMultiple float64
	// TrailStepMultiple scales observed positive PnL steps into trailing distance.
	TrailStepMultiple float64
	// LiquidityStepMultiple determines how much pullback is tolerated when the
	// best bid alone cannot absorb the full position.
	LiquidityStepMultiple float64
	// StepAlpha controls how quickly the observed positive-step estimate adapts.
	StepAlpha float64
}

// DefaultAdaptiveExitConfig provides an initial, deliberately small parameter
// set for virtual-position experiments. It replaces fixed +PnL tiers with
// multipliers of observed movement and measured round-trip friction.
func DefaultAdaptiveExitConfig() AdaptiveExitConfig {
	return AdaptiveExitConfig{
		LossFrictionMultiple:  2.0,
		LossStepMultiple:      3.0,
		TrailFrictionMultiple: 1.0,
		TrailStepMultiple:     2.0,
		LiquidityStepMultiple: 1.0,
		StepAlpha:             0.25,
	}
}

// ExitObservation is one mark-to-liquidation observation from a position.
type ExitObservation struct {
	// PnLPct is net PnL in percentage points, as supplied by Position.PnLPct.
	PnLPct float64
	// FrictionPct is the round-trip spread + fees, in percentage points.
	FrictionPct float64
	// ExitRoom is best-bid quantity / position quantity.
	ExitRoom float64
	HasExitRoom bool
}

// ExitDecision contains the policy result and diagnostics suitable for tests
// and for later comparison with historical virtual-position outcomes.
type ExitDecision struct {
	Exit             bool
	Reason           string
	LiquidityStress  bool
	TrailArmed       bool
	PeakPnLPct       float64
	LowestPnLPct     float64
	LossFloorPct     float64
	TrailFloorPct    float64
	TypicalUpStepPct float64
}

// AdaptiveExitPolicy adapts its per-position thresholds to observed movement.
// It intentionally has no time-based exit and no fixed +5/+15/+30/+60% tiers.
type AdaptiveExitPolicy struct {
	config AdaptiveExitConfig

	initialized      bool
	lastPnLPct       float64
	peakPnLPct       float64
	lowestPnLPct     float64
	typicalUpStepPct float64
	upSteps          int

	lossFloorPct  float64
	trailFloorPct float64
	trailArmed    bool
}

// NewAdaptiveExitPolicy creates an independent policy instance for one position.
func NewAdaptiveExitPolicy(config AdaptiveExitConfig) *AdaptiveExitPolicy {
	defaults := DefaultAdaptiveExitConfig()
	if config.LossFrictionMultiple <= 0 {
		config.LossFrictionMultiple = defaults.LossFrictionMultiple
	}
	if config.LossStepMultiple <= 0 {
		config.LossStepMultiple = defaults.LossStepMultiple
	}
	if config.TrailFrictionMultiple <= 0 {
		config.TrailFrictionMultiple = defaults.TrailFrictionMultiple
	}
	if config.TrailStepMultiple <= 0 {
		config.TrailStepMultiple = defaults.TrailStepMultiple
	}
	if config.LiquidityStepMultiple <= 0 {
		config.LiquidityStepMultiple = defaults.LiquidityStepMultiple
	}
	if config.StepAlpha <= 0 || config.StepAlpha > 1 {
		config.StepAlpha = defaults.StepAlpha
	}
	return &AdaptiveExitPolicy{config: config}
}

// Evaluate ingests one observation and returns an exit decision, if any.
//
// The policy estimates typical movement from positive PnL steps. While the
// trailing policy has not armed, the initial risk floor can widen as evidence
// of movement accumulates, but never beyond HardLossLimitPct when configured.
// Once trailing arms, its floor can only move up; it never gives back protection.
func (policy *AdaptiveExitPolicy) Evaluate(observation ExitObservation) ExitDecision {
	if policy == nil || math.IsNaN(observation.PnLPct) || math.IsInf(observation.PnLPct, 0) {
		return ExitDecision{}
	}

	friction := observation.FrictionPct
	if friction <= 0 || math.IsNaN(friction) || math.IsInf(friction, 0) {
		friction = 1.6
	}
	current := observation.PnLPct

	if !policy.initialized {
		policy.initialized = true
		policy.lastPnLPct = current
		policy.peakPnLPct = math.Max(0, current)
		policy.lowestPnLPct = math.Min(0, current)
	} else {
		delta := current - policy.lastPnLPct
		if delta > 0 {
			if policy.upSteps == 0 {
				policy.typicalUpStepPct = delta
			} else {
				policy.typicalUpStepPct += policy.config.StepAlpha * (delta - policy.typicalUpStepPct)
			}
			policy.upSteps++
		}
		if current > policy.peakPnLPct {
			policy.peakPnLPct = current
		}
		if current < policy.lowestPnLPct {
			policy.lowestPnLPct = current
		}
		policy.lastPnLPct = current
	}

	riskDistance := math.Max(
		friction*policy.config.LossFrictionMultiple,
		policy.typicalUpStepPct*policy.config.LossStepMultiple,
	)
	if policy.config.HardLossLimitPct > 0 {
		riskDistance = math.Min(riskDistance, policy.config.HardLossLimitPct)
	}
	if !policy.trailArmed {
		policy.lossFloorPct = -riskDistance
	}

	trailDistance := math.Max(
		friction*policy.config.TrailFrictionMultiple,
		policy.typicalUpStepPct*policy.config.TrailStepMultiple,
	)
	// Require more than one observed rising step before trusting the trail.
	// A single jump immediately after entry is not enough to estimate noise.
	if !policy.trailArmed && policy.upSteps >= 2 &&
		policy.peakPnLPct > friction+trailDistance {
		policy.trailArmed = true
		policy.trailFloorPct = policy.peakPnLPct - trailDistance
	}
	if policy.trailArmed {
		candidateFloor := policy.peakPnLPct - trailDistance
		if candidateFloor > policy.trailFloorPct {
			policy.trailFloorPct = candidateFloor
		}
	}

	decision := ExitDecision{
		TrailArmed:       policy.trailArmed,
		PeakPnLPct:       policy.peakPnLPct,
		LowestPnLPct:     policy.lowestPnLPct,
		LossFloorPct:     policy.lossFloorPct,
		TrailFloorPct:    policy.trailFloorPct,
		TypicalUpStepPct: policy.typicalUpStepPct,
	}

	// ExitRoom below one means the top bid alone cannot absorb the whole lot.
	// That fact does not prove the entire book is empty, so require a material
	// pullback as well and use the full-depth PnL mark supplied by Position.
	if observation.HasExitRoom && observation.ExitRoom > 0 &&
		observation.ExitRoom < 1 && current < policy.peakPnLPct {
		liquidityDistance := math.Max(
			friction,
			policy.typicalUpStepPct*policy.config.LiquidityStepMultiple,
		)
		if policy.peakPnLPct-current >= liquidityDistance {
			decision.Exit = true
			decision.LiquidityStress = true
			decision.Reason = fmt.Sprintf(
				"adaptive liquidity stress: exit room %.3f, peak +%.2f%%, current +%.2f%%, pullback %.2f%%",
				observation.ExitRoom, policy.peakPnLPct, current, policy.peakPnLPct-current,
			)
			return decision
		}
	}

	if current <= policy.lossFloorPct {
		decision.Exit = true
		decision.Reason = fmt.Sprintf(
			"adaptive invalidation: net PnL %.2f%% reached dynamic floor %.2f%%",
			current, policy.lossFloorPct,
		)
		return decision
	}
	if policy.trailArmed && current <= policy.trailFloorPct {
		decision.Exit = true
		decision.Reason = fmt.Sprintf(
			"adaptive trailing exit: peak +%.2f%%, floor +%.2f%%, current +%.2f%%",
			policy.peakPnLPct, policy.trailFloorPct, current,
		)
		return decision
	}
	return decision
}

