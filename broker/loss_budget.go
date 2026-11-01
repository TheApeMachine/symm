package broker

import "github.com/theapemachine/symm/system"

/*
LossBudget freezes the invalidation width at entry from the symbol's
friction. Wide microcap spreads get room to breathe; a hard ceiling keeps
a small bankroll from taking unrecoverable cuts.
*/
type LossBudget struct {
	armed   bool
	maxLoss float64
	armPct  float64
}

func NewLossBudget() *LossBudget {
	return &LossBudget{}
}

/*
Arm locks max loss and trail-arm from friction (fraction of mid). Re-arming
is a no-op so later spread flicker cannot chase the floor wider.
*/
func (budget *LossBudget) Arm(frictionFraction float64) *LossBudget {
	if budget == nil {
		return nil
	}

	if budget.armed {
		return budget
	}

	maxLoss := 2.0
	arm := 0.5
	mult := 2.0
	ceiling := 4.0

	if system.Cfg != nil && system.Cfg.Risk != nil {
		risk := system.Cfg.Risk

		if risk.MaxAllowedLossPct > 0 {
			maxLoss = risk.MaxAllowedLossPct
		}

		if risk.TrailPeakArmPct > 0 {
			arm = risk.TrailPeakArmPct
		}

		if risk.LossFrictionMult > 0 {
			mult = risk.LossFrictionMult
		}

		if risk.MaxInvalidationPct > 0 {
			ceiling = risk.MaxInvalidationPct
		}
	}

	if frictionFraction > 0 {
		frictionPts := frictionFraction * 100
		fromFriction := frictionPts * mult

		if fromFriction > maxLoss {
			maxLoss = fromFriction
		}

		// Trail waits until the mark clears one friction of noise.
		if frictionPts > arm {
			arm = frictionPts
		}
	}

	if maxLoss > ceiling {
		maxLoss = ceiling
	}

	budget.maxLoss = maxLoss
	budget.armPct = arm
	budget.armed = true

	return budget
}

func (budget *LossBudget) Armed() bool {
	return budget != nil && budget.armed
}

func (budget *LossBudget) MaxLoss() float64 {
	if budget == nil || !budget.armed {
		return defaultMaxLoss()
	}

	return budget.maxLoss
}

func (budget *LossBudget) ArmPct() float64 {
	if budget == nil || !budget.armed {
		return defaultTrailArm()
	}

	return budget.armPct
}

/*
Prefer overlays learned rail preferences after Arm. Still respects the
configured invalidation ceiling so ruin math cannot drift unbounded.
*/
func (budget *LossBudget) Prefer(maxLoss, armPct float64) *LossBudget {
	if budget == nil {
		return nil
	}

	if !budget.armed {
		budget.Arm(0)
	}

	ceiling := 4.0
	if system.Cfg != nil && system.Cfg.Risk != nil && system.Cfg.Risk.MaxInvalidationPct > 0 {
		ceiling = system.Cfg.Risk.MaxInvalidationPct
	}

	if maxLoss > 0 {
		if maxLoss > ceiling {
			maxLoss = ceiling
		}

		budget.maxLoss = maxLoss
	}

	if armPct > 0 {
		budget.armPct = armPct
	}

	return budget
}

func defaultMaxLoss() float64 {
	if system.Cfg != nil && system.Cfg.Risk != nil && system.Cfg.Risk.MaxAllowedLossPct > 0 {
		return system.Cfg.Risk.MaxAllowedLossPct
	}

	return 2.0
}

func defaultTrailArm() float64 {
	if system.Cfg != nil && system.Cfg.Risk != nil && system.Cfg.Risk.TrailPeakArmPct > 0 {
		return system.Cfg.Risk.TrailPeakArmPct
	}

	return 0.5
}
