package strategy

import (
	"sort"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

/*
admit selects entries by utility while keeping reserved overflow opportunity-
only. When free slots are exhausted, a challenger may displace the weakest
incumbent only if rotate surplus is positive.
*/
func (planner *Planner) admit(
	thesis *types.Thesis,
	entries []types.Decision,
	freeNormal int,
	freeReserved int,
	incumbents []Incumbent,
) {
	sort.Slice(entries, func(left, right int) bool {
		return entries[left].Utility > entries[right].Utility
	})

	admittedNormal := 0
	admittedReserved := 0

	for _, decision := range entries {
		opportunity := decision.AllocationClass == "reserved"
		useNormal := admittedNormal < freeNormal
		useReserved := opportunity && admittedReserved < freeReserved

		if useNormal || useReserved {
			if useNormal {
				admittedNormal++
			}

			if !useNormal && useReserved {
				admittedReserved++
			}

			planner.persistAcceptedEntry(thesis, decision, opportunity)

			continue
		}

		if planner.displace(thesis, &decision, incumbents) {
			continue
		}

		planner.persistRejectedEntry(
			thesis, decision, opportunity, freeNormal, admittedNormal, incumbents,
		)
	}
}

/*
displace replaces the weakest open holding when the challenger's enter utility
clears hold utility plus exit friction. Sizes the challenger from the capital
the exit frees.
*/
func (planner *Planner) displace(
	thesis *types.Thesis,
	decision *types.Decision,
	incumbents []Incumbent,
) bool {
	index, found := weakest(incumbents)

	if !found {
		return false
	}

	incumbent := &incumbents[index]
	surplus := rotateSurplus(
		decision.Utility, incumbent.HoldUtility, incumbent.ExitCost,
	)

	if surplus <= 0 {
		return false
	}

	if incumbent.Notional <= 0 {
		return false
	}

	planner.scaleTo(decision, incumbent.Notional)
	incumbent.Displaced = true

	decision.Cause = "rotation"
	decision.Reason = "challenger utility clears weakest incumbent after exit cost"
	decision.Alternatives["hold_incumbent"] = incumbent.HoldUtility
	decision.Alternatives["exit_cost"] = incumbent.ExitCost
	decision.Alternatives["rotate_surplus"] = surplus

	thesis.Decisions = append(thesis.Decisions, types.Decision{
		Action:            "exit",
		Symbol:            incumbent.Symbol,
		At:                decision.At,
		Utility:           -incumbent.ExitCost,
		Alternatives:      map[string]float64{"exit": -incumbent.ExitCost, "hold": incumbent.HoldUtility},
		ProposedQuantity:  incumbent.Qty,
		ReferencePrice:    incumbent.Mark,
		ValidThroughEpoch: decision.ValidThroughEpoch,
		Cause:             "rotation",
		Reason:            "displaced by higher-utility challenger " + decision.Symbol,
	})

	thesis.Lifecycle.Store(incumbent.Symbol, types.LifecycleExitSelected)

	if planner.instrument != nil {
		pair, err := planner.instrument.Pair(incumbent.Symbol)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"failed to get instrument pair",
				err,
			))

			return false
		}

		thesis.Positions.Store(incumbent.Symbol, broker.NewPosition(
			planner.api,
			planner.instrument,
			planner.price,
			planner.balance,
			pair,
		))
	}

	planner.persistAcceptedEntry(
		thesis, *decision, decision.AllocationClass == "reserved",
	)

	return true
}

/*
scaleTo sets proposed notional to the capital freed by a displaced incumbent
and rescales quantity so the enter lot matches that budget. Cash-capped
counterfactual proposals may be smaller than the exit frees; rotate must
scale up to the freed notional, not only down.
*/
func (planner *Planner) scaleTo(decision *types.Decision, notional float64) {
	if notional <= 0 || decision.ProposedNotional <= 0 {
		return
	}

	scale := notional / decision.ProposedNotional
	decision.ProposedNotional = notional
	decision.ProposedQuantity *= scale
}

/*
persistRejectedEntry records a slot-exhausted or rotate-rejected entry as an
explicit nothing decision so the Thesis audit trail shows why utility-ranked
intent did not execute.
*/
func (planner *Planner) persistRejectedEntry(
	thesis *types.Thesis,
	decision types.Decision,
	opportunity bool,
	freeNormal int,
	admittedNormal int,
	incumbents []Incumbent,
) {
	decision.Action = "nothing"
	prior := decision.Utility
	decision.Utility = 0
	decision.Cause = "slots_full"
	decision.Reason = "higher-utility entries consumed available slots"

	if !opportunity && freeNormal <= admittedNormal {
		decision.Reason = "normal slots full; reserved requires opportunity"
	}

	if index, found := weakest(incumbents); found {
		incumbent := incumbents[index]
		surplus := rotateSurplus(prior, incumbent.HoldUtility, incumbent.ExitCost)
		decision.Alternatives["hold_incumbent"] = incumbent.HoldUtility
		decision.Alternatives["exit_cost"] = incumbent.ExitCost
		decision.Alternatives["rotate_surplus"] = surplus

		if surplus <= 0 {
			decision.Cause = "rotate_wait"
			decision.Reason = "challenger utility does not clear weakest incumbent after exit cost"
		}
	}

	thesis.Decisions = append(thesis.Decisions, decision)
}

/*
persistAcceptedEntry writes lifecycle, decisions, positions, and orders for
one admitted entry so downstream broker and UI layers see the full executable
intent.
*/
func (planner *Planner) persistAcceptedEntry(
	thesis *types.Thesis,
	decision types.Decision,
	opportunity bool,
) {
	thesis.Lifecycle.Store(decision.Symbol, types.LifecycleEntrySelected)
	thesis.Decisions = append(thesis.Decisions, decision)

	holding := types.NewHolding(
		planner.ctx,
		decision.Symbol,
		decimal.NewFromFloat64(decision.ProposedQuantity),
	)
	holding.IsOpportunity = opportunity
	thesis.Holdings.Store(decision.Symbol, holding)

	if planner.instrument == nil {
		return
	}

	pair, err := planner.instrument.Pair(decision.Symbol)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get instrument pair",
			err,
		))

		return
	}

	thesis.Positions.Store(decision.Symbol, broker.NewPosition(
		planner.api,
		planner.instrument,
		planner.price,
		planner.balance,
		pair,
	))
}

/*
entry computes executable utility for opening one slot and caps proposed
notional at both wallet cash and visible best-ask capacity. Utility is the
single economic gate — expected return minus uncertainty minus round-trip
friction. Cognition must still clear the forecast noise share;
AllocationClass reserved further needs Opportunity.Reserved — strong SNR,
noise-clearing cognitive lead, next-event horizon, and non-ambiguous contrast.
*/
func (planner *Planner) entry(
	thesis *types.Thesis,
	forecast types.Forecasts,
	cognition types.Cognition,
	fee float64,
	capital float64,
	available float64,
) types.Decision {
	// BuyCapacity is a liquidity ceiling, not a wallet. Proposed notional must
	// stay inside deployable cash so Fraction (proposed/available) stays honest.
	proposed := min(capital, forecast.BuyCapacity)

	if available > 0 {
		proposed = min(proposed, available)
	}

	unitCost := forecast.ReferencePrice * (1 + forecast.ExpectedSpread/2) * (1 + fee)
	quantity := 0.0

	if unitCost > 0 {
		quantity = proposed / unitCost
	}

	reading := measureOpportunity(forecast, cognition, thesis)
	utility := reading.Margin - 2*fee - forecast.ExpectedSpread -
		forecast.ExpectedImpact - forecast.ExpectedAdverseSelection

	if proposed <= 0 || utility <= 0 {
		return planner.rejectEntry(
			forecast, reading, utility, proposed, quantity, fee,
			"infeasible",
			"expected executable utility does not exceed doing nothing",
		)
	}

	if !reading.CognitiveClears(forecast) {
		return planner.rejectEntry(
			forecast, reading, utility, proposed, quantity, fee,
			"cognitive_weak",
			"cognitive confidence does not clear forecast noise share",
		)
	}

	allocation := "normal"

	if reading.Reserved() {
		allocation = "reserved"
	}

	return types.Decision{
		Action:            "enter",
		Symbol:            forecast.Symbol,
		At:                forecast.At,
		Utility:           utility,
		Alternatives:      map[string]float64{"enter": utility, "nothing": 0},
		AllocationClass:   allocation,
		ProposedNotional:  proposed,
		ProposedQuantity:  quantity,
		ExpectedFees:      2 * fee,
		ExpectedSpread:    forecast.ExpectedSpread,
		ReferencePrice:    forecast.ReferencePrice,
		ValidThroughEpoch: forecast.ExpiresEpoch,
		ForecastSource:    forecast.Source,
		OpportunityMargin: reading.Margin,
		CognitiveLead:     reading.Lead,
		BasinConfidence:   reading.Basin,
		Cause:             "entry",
		Reason:            "executable utility exceeds doing nothing",
	}
}

/*
rejectEntry records a nothing decision that still exposes the opportunity
surface so audit and UI can see why enter was refused.
*/
func (planner *Planner) rejectEntry(
	forecast types.Forecasts,
	reading Opportunity,
	utility, proposed, quantity, fee float64,
	cause, reason string,
) types.Decision {
	decision := planner.nothing(forecast, reason)
	decision.Cause = cause
	decision.Utility = utility
	decision.Alternatives["enter"] = utility
	decision.ProposedNotional = proposed
	decision.ProposedQuantity = quantity
	decision.ExpectedFees = 2 * fee
	decision.ExpectedSpread = forecast.ExpectedSpread
	decision.OpportunityMargin = reading.Margin
	decision.CognitiveLead = reading.Lead
	decision.BasinConfidence = reading.Basin

	return decision
}

/*
nothing records an explicit no-action selection while retaining the forecast
price and validity boundary that made the comparison possible.
*/
func (planner *Planner) nothing(
	forecast types.Forecasts,
	reason string,
) types.Decision {
	return types.Decision{
		Action:            "nothing",
		Symbol:            forecast.Symbol,
		At:                forecast.At,
		Alternatives:      map[string]float64{"nothing": 0},
		ReferencePrice:    forecast.ReferencePrice,
		ValidThroughEpoch: forecast.ExpiresEpoch,
		ForecastSource:    forecast.Source,
		Cause:             "infeasible",
		Reason:            reason,
	}
}

/*
context records the forecast decomposition and portfolio values actually used
for one utility comparison so the Decision remains auditable on its Thesis.
*/
func (planner *Planner) context(
	decision *types.Decision,
	forecast types.Forecasts,
	available float64,
	openPositions int,
	slots int,
) {
	decision.ForecastModel = forecast.ModelVersion
	decision.ForecastEpoch = forecast.SourceEpoch
	decision.CalibrationCount = forecast.CalibrationSamples
	decision.ExpectedReturn = forecast.ExpectedReturn
	decision.ExpectedImpact = forecast.ExpectedImpact
	decision.AdverseSelection = forecast.ExpectedAdverseSelection
	decision.Uncertainty = forecast.Uncertainty
	decision.Confidence = forecast.Confidence
	decision.AvailableCapital = available
	decision.OpenPositions = openPositions
	decision.SlotCapacity = slots
}
