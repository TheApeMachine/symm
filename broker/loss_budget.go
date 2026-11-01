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
	if budget.armed {
		return budget
	}

	risk := system.Cfg.Risk
	maxLoss := risk.MaxAllowedLossPct
	arm := risk.TrailPeakArmPct
	mult := risk.LossFrictionMult
	ceiling := risk.MaxInvalidationPct

	if frictionFraction > 0 {
		frictionPts := frictionFraction * 100
		fromFriction := frictionPts * mult

		if fromFriction > maxLoss {
			maxLoss = fromFriction
		}

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

/*
Prefer overlays learned rail preferences after Arm. Still respects the
configured invalidation ceiling so ruin math cannot drift unbounded.
*/
func (budget *LossBudget) Prefer(maxLoss, armPct float64) {
	ceiling := system.Cfg.Risk.MaxInvalidationPct

	if maxLoss > 0 {
		if maxLoss > ceiling {
			maxLoss = ceiling
		}

		budget.maxLoss = maxLoss
	}

	if armPct > 0 {
		budget.armPct = armPct
	}
}
