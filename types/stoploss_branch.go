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

	matches := false

	switch branch.When {
	case StopAlways:
		matches = true
	case StopBookInvalid:
		if stoploss.isReflexiveCascade() {
			matches = false
		} else {
			matches = surface != nil && !surface.BookComplete && stoploss.BookObserved
		}
	case StopFloorUncovered:
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
		if stoploss.isSweepProtected(mark) {
			matches = false
		} else {
			matches = mark != nil && stoploss.Floor != nil &&
				mark.Cmp(stoploss.Floor) <= 0
		}
	case StopTrailingFloor:
		matches = stoploss.Locked && stoploss.LockFloor != nil &&
			stoploss.Floor != nil && stoploss.Floor.Cmp(stoploss.LockFloor) > 0 &&
			mark != nil && mark.Cmp(stoploss.LockFloor) > 0
	case StopProtectedFloor:
		matches = stoploss.Locked
	case StopMomentumLost:
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

		if !matches && mark != nil && stoploss.SurgeArmed && stoploss.MomentumFloor != nil &&
			stoploss.MomentumFloor.Sign() > 0 && stoploss.Peak != nil {
			momentumLine := floorToTick(
				scaled(stoploss.Peak).Sub(stoploss.MomentumFloor),
				stoploss.TickSize,
			)

			matches = momentumLine != nil && mark.Cmp(momentumLine) <= 0
		}
	case StopClimaxExhaustion:
		matches = stoploss.isClimaxExhausted(mark)
	case StopProfitStagnated:
		confirmMarks := stoploss.ConfirmMarks

		if confirmMarks < 1 {
			confirmMarks = 3
		}

		if mark != nil && stoploss.ProfitLatched && stoploss.ProfitLine != nil &&
			stoploss.Peak != nil && mark.Cmp(stoploss.ProfitLine) > 0 &&
			!stoploss.isParabolicRun() &&
			stoploss.DistinctNonPeakMarks >= confirmMarks {
			giveback := scaled(stoploss.Peak).Sub(scaled(mark))
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
		if child.Resolve(stoploss, surface, mark) {
			return true
		}
	}

	switch branch.Then {
	case StopNoAction:
		return false
	case StopInvalidateRegime:
		stoploss.Status = TRIGGERED
		stoploss.TriggerReason = TriggerRegimeInvalidated

		if surface.ExecutableVWAP != nil && surface.ExecutableVWAP.Sign() > 0 {
			stoploss.TriggerMark = scaled(surface.ExecutableVWAP)
			return true
		}

		stoploss.TriggerMark = scaled(stoploss.Mark)
	case StopExitHardFloor:
		stoploss.Status = TRIGGERED
		stoploss.TriggerReason = TriggerHardFloor
		stoploss.TriggerMark = mark
	case StopExitProtectedFloor:
		stoploss.Status = TRIGGERED
		stoploss.TriggerReason = TriggerProtectedFloor
		stoploss.TriggerMark = mark
	case StopExitTrailingFloor:
		stoploss.Status = TRIGGERED
		stoploss.TriggerReason = TriggerTrailingFloor
		stoploss.TriggerMark = mark
	case StopExitClimax:
		stoploss.Status = TRIGGERED
		stoploss.TriggerReason = TriggerClimaxExhaustion
		stoploss.TriggerMark = mark
	case StopExitMomentum:
		stoploss.Status = TRIGGERED
		stoploss.TriggerReason = TriggerPumpMomentumLost
		stoploss.TriggerMark = mark
	case StopExitStagnation:
		stoploss.Status = TRIGGERED
		stoploss.TriggerReason = TriggerProfitStagnation
		stoploss.TriggerMark = mark
	default:
		panic("stoploss: unknown branch action")
	}

	return true
}
