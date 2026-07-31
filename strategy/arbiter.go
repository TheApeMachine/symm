package strategy

import (
	"math"
	"sort"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

type Incumbent struct {
	Symbol      string
	HoldUtility float64
	ExitCost    float64
	Notional    *decimal.Decimal
	Qty         *decimal.Decimal
	Mark        *decimal.Decimal
}

type Arbiter struct {
	desk  *broker.Desk
	price *broker.Price
}

func NewArbiter(desk *broker.Desk, price *broker.Price) *Arbiter {
	return &Arbiter{
		desk:  desk,
		price: price,
	}
}

// Arbitrate ranks candidate entries, allocates available slots, and performs displacement/rotation.
func (a *Arbiter) Arbitrate(thesis *types.Thesis) {
	if thesis == nil {
		return
	}

	enters := make([]types.Decision, 0, len(thesis.Decisions))
	retained := make([]types.Decision, 0, len(thesis.Decisions))

	for _, d := range thesis.Decisions {
		if d.Action == types.ActionEnter {
			enters = append(enters, d)
		} else {
			retained = append(retained, d)
		}
	}

	// Sort enter candidates by Utility descending
	sort.Slice(enters, func(i, j int) bool {
		return enters[i].Utility > enters[j].Utility
	})

	openSlots := a.desk.OpenSlots(false)
	incumbents := a.getIncumbents(thesis)

	for _, candidate := range enters {
		if openSlots > 0 {
			thesis.NoteLifecycle(candidate.Symbol, types.LifecycleEntrySelected, thesis.At)
			retained = append(retained, candidate)
			openSlots--
			continue
		}

		// Slots exhausted: Check Bellman Rotation Gate against weakest incumbent
		if exitDec, enterDec, ok := a.tryDisplace(candidate, incumbents); ok {
			thesis.NoteLifecycle(exitDec.Symbol, types.LifecycleExitSelected, thesis.At)
			thesis.NoteLifecycle(enterDec.Symbol, types.LifecycleEntrySelected, thesis.At)
			retained = append(retained, exitDec, enterDec)
			continue
		}

		// Rejection due to full slots and insufficient rotation edge
		candidate.Action = types.ActionNothing
		candidate.Cause = "slots_full"
		candidate.Reason = "insufficient utility to displace active incumbents"
		retained = append(retained, candidate)
	}

	thesis.Decisions = retained
}

func (a *Arbiter) tryDisplace(
	candidate types.Decision,
	incumbents []Incumbent,
) (types.Decision, types.Decision, bool) {
	for i, inc := range incumbents {
		// Bellman Rotation Gate: Candidate Utility > Holding Utility + Exit Cost
		surplus := candidate.Utility - inc.HoldUtility - inc.ExitCost
		if surplus <= 0 {
			continue
		}

		incumbents[i].HoldUtility = math.Inf(1) // Prevent double displacement

		exitDec := types.Decision{
			Action:           types.ActionExit,
			Symbol:           inc.Symbol,
			At:               candidate.At,
			Utility:          -inc.ExitCost,
			ProposedQuantity: inc.Qty.Copy(),
			ReferencePrice:   inc.Mark.Copy(),
			Cause:            "rotation",
			Reason:           "displaced by higher-utility challenger " + candidate.Symbol,
		}

		candidate.Cause = "rotation"
		candidate.Displaces = inc.Symbol
		candidate.ProposedNotional = inc.Notional.Copy()
		candidate.DisplacedQuantity = inc.Qty.Copy()
		candidate.DisplacedPrice = inc.Mark.Copy()
		candidate.Reason = "rotates out weaker incumbent " + inc.Symbol

		return exitDec, candidate, true
	}

	return types.Decision{}, types.Decision{}, false
}

func (a *Arbiter) getIncumbents(thesis *types.Thesis) []Incumbent {
	rows := make([]Incumbent, 0)
	for position := range a.desk.Positions() {
		if position.Status != types.OPEN || position.Holding.Mark == nil || position.Holding.Qty == nil {
			continue
		}

		notional := decimal.ExactMul(position.Holding.Mark, position.Holding.Qty)
		if notional == nil || notional.Sign() <= 0 {
			continue
		}

		feeFraction := 0.0026
		if fraction, err := a.price.Fraction(position.Holding.Symbol); err == nil && fraction != nil {
			feeFraction = fraction.Float64()
		}

		rows = append(rows, Incumbent{
			Symbol: position.Holding.Symbol,
			HoldUtility: func() float64 {
				if position.Holding.ReturnPct != nil {
					return *position.Holding.ReturnPct
				}
				return 0.0
			}(),
			ExitCost: feeFraction * notional.Float64(),
			Notional: notional,
			Qty:      position.Holding.Qty.Copy(),
			Mark:     position.Holding.Mark.Copy(),
		})
	}
	return rows
}
