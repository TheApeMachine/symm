package types

import "github.com/krakenfx/api-go/v2/pkg/decimal"

/*
StopCondition names one terminal protection decision in the Stoploss tree.
*/
type StopCondition uint8

const (
	StopAlways StopCondition = iota
	StopBookInvalid
	StopFloorUncovered
	StopFloorBreached
	StopTrailingFloor
	StopProtectedFloor
	StopMomentumLost
	StopProfitStagnated
	StopClimaxExhaustion
)

/*
StopAction names the exit claimed by a matching terminal branch.
*/
type StopAction uint8

const (
	StopNoAction StopAction = iota
	StopInvalidateRegime
	StopExitHardFloor
	StopExitProtectedFloor
	StopExitTrailingFloor
	StopExitMomentum
	StopExitStagnation
	StopExitClimax
)

/*
Branch is one readable Stoploss decision. Matching branches resolve their more
specific children first, then apply their own action as the fallback.
*/
type Branch struct {
	When     StopCondition
	Then     StopAction
	Branches []*Branch
}

var stoplossBranches = &Branch{
	When: StopAlways,
	Branches: []*Branch{
		{When: StopBookInvalid, Then: StopInvalidateRegime},
		{When: StopFloorUncovered, Then: StopInvalidateRegime},
		{
			When: StopFloorBreached,
			Then: StopExitHardFloor,
			Branches: []*Branch{
				{When: StopTrailingFloor, Then: StopExitTrailingFloor},
				{When: StopProtectedFloor, Then: StopExitProtectedFloor},
			},
		},
		{When: StopClimaxExhaustion, Then: StopExitClimax},
		{When: StopMomentumLost, Then: StopExitMomentum},
		{When: StopProfitStagnated, Then: StopExitStagnation},
	},
}

/*
Resolve walks the Stoploss decision tree and applies the first terminal action.
*/
func (branch *Branch) Resolve(
	stoploss *Stoploss,
	surface *ExecutionSurface,
	mark *decimal.Decimal,
) bool {
	if branch == nil || stoploss == nil || stoploss.Status == TRIGGERED {
		return false
	}

	currentMark := mark

	if (currentMark == nil || currentMark.Sign() <= 0) && surface != nil {
		if surface.ExecutableVWAP != nil && surface.ExecutableVWAP.Sign() > 0 {
			currentMark = surface.ExecutableVWAP
		} else if surface.BestBid != nil && surface.BestBid.Sign() > 0 {
			currentMark = surface.BestBid
		}
	}

	if currentMark == nil || currentMark.Sign() <= 0 {
		currentMark = stoploss.Mark
	}

	matches := false

	switch branch.When {
	case StopAlways:
		matches = true
	case StopBookInvalid:
		if !stoploss.InProfit(currentMark) {
			matches = false
			break
		}

		if stoploss.isReflexiveCascade() {
			matches = false
		} else {
			matches = surface != nil && !surface.BookComplete && stoploss.BookObserved
		}
	case StopFloorUncovered:
		if !stoploss.InProfit(currentMark) {
			matches = false
			break
		}

		if stoploss.isReflexiveCascade() {
			matches = false
		} else {
			matches = surface != nil && surface.BookComplete &&
				surface.FullyExecutable && surface.ExecutableVWAP != nil &&
				surface.ExecutableVWAP.Sign() > 0 && stoploss.Locked &&
				surface.FloorCoverageQty != nil && surface.SellableQty != nil &&
				surface.FloorCoverageQty.Cmp(surface.SellableQty) < 0
		}
	case StopFloorBreached:
		if stoploss.isSweepProtected(currentMark) {
			matches = false
		} else {
			matches = currentMark != nil && stoploss.Floor != nil &&
				currentMark.Cmp(stoploss.Floor) <= 0
		}
	case StopTrailingFloor:
		matches = stoploss.Locked && stoploss.LockFloor != nil &&
			stoploss.Floor != nil && stoploss.Floor.Cmp(stoploss.LockFloor) > 0 &&
			currentMark != nil && currentMark.Cmp(stoploss.LockFloor) > 0
	case StopProtectedFloor:
		matches = stoploss.Locked
	case StopMomentumLost:
		if !stoploss.InProfit(currentMark) {
			matches = false
			break
		}

		// The advisors speak for every position, not only for entries the old
		// precursor layer tagged. That layer is gone, and gating this exit on
		// its flag left the momentum-decay path unreachable: a stalling or
		// reversing council could no longer close anything.
		if momentum, ok := stoploss.Causative.ActivePerspectives["momentum"]; ok {
			if momentum == "Stalling" || momentum == "Reversing" {
				matches = true
			}
		}

		if profitRun, ok := stoploss.Causative.ActivePerspectives["profit_run"]; ok {
			if profitRun == "Exhausting" || profitRun == "GivingBack" {
				matches = true
			}
		}

		if !matches && currentMark != nil && stoploss.SurgeArmed && stoploss.MomentumFloor != nil &&
			stoploss.MomentumFloor.Sign() > 0 && stoploss.Peak != nil {
			momentumLine := floorToTick(
				scaled(stoploss.Peak).Sub(stoploss.MomentumFloor),
				stoploss.TickSize,
			)

			matches = momentumLine != nil && currentMark.Cmp(momentumLine) <= 0
		}
	case StopClimaxExhaustion:
		matches = stoploss.isClimaxExhausted(currentMark)
	case StopProfitStagnated:
		confirmMarks := stoploss.ConfirmMarks

		if confirmMarks < 1 {
			confirmMarks = 3
		}

		if currentMark != nil && stoploss.ProfitLatched && stoploss.ProfitLine != nil &&
			stoploss.Peak != nil && currentMark.Cmp(stoploss.ProfitLine) > 0 &&
			!stoploss.isParabolicRun() &&
			stoploss.DistinctNonPeakMarks >= confirmMarks {
			giveback := scaled(stoploss.Peak).Sub(scaled(currentMark))
			matches = giveback.Cmp(stoploss.stagnationTolerance()) >= 0

			if !matches {
				if profitRun, ok := stoploss.Causative.ActivePerspectives["profit_run"]; ok {
					if profitRun == "GivingBack" {
						matches = true
					}
				}
			}
		}
	default:
		panic("stoploss: unknown branch condition")
	}

	if !matches {
		return false
	}

	for _, child := range branch.Branches {
		if child.Resolve(stoploss, surface, currentMark) {
			return true
		}
	}

	switch branch.Then {
	case StopNoAction:
		return false
	case StopInvalidateRegime:
		stoploss.Status = TRIGGERED
		stoploss.TriggerReason = TriggerRegimeInvalidated
		stoploss.TriggerMark = currentMark
	case StopExitHardFloor:
		stoploss.Status = TRIGGERED
		stoploss.TriggerReason = TriggerHardFloor
		stoploss.TriggerMark = currentMark
	case StopExitProtectedFloor:
		stoploss.Status = TRIGGERED
		stoploss.TriggerReason = TriggerProtectedFloor
		stoploss.TriggerMark = currentMark
	case StopExitTrailingFloor:
		stoploss.Status = TRIGGERED
		stoploss.TriggerReason = TriggerTrailingFloor
		stoploss.TriggerMark = currentMark
	case StopExitClimax:
		stoploss.Status = TRIGGERED
		stoploss.TriggerReason = TriggerClimaxExhaustion
		stoploss.TriggerMark = currentMark
	case StopExitMomentum:
		stoploss.Status = TRIGGERED
		stoploss.TriggerReason = TriggerPumpMomentumLost
		stoploss.TriggerMark = currentMark
	case StopExitStagnation:
		stoploss.Status = TRIGGERED
		stoploss.TriggerReason = TriggerProfitStagnation
		stoploss.TriggerMark = currentMark
	default:
		panic("stoploss: unknown branch action")
	}

	return true
}
