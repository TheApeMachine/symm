package strategy

import (
	"context"
	"maps"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

/*
Admit records accepted and rejected entry decisions onto Thesis and sizes
rotation redeploys from freed incumbent capital.
*/
type Admit struct {
	ctx     context.Context
	balance *broker.Balance
	desk    *broker.Desk
	rotate  Rotate
	quote   string
}

/*
NewAdmit wires wallet/desk surfaces used when persisting enter decisions.
*/
func NewAdmit(
	ctx context.Context,
	balance *broker.Balance,
	desk *broker.Desk,
	rotate Rotate,
) *Admit {
	return &Admit{
		ctx:     ctx,
		balance: balance,
		desk:    desk,
		rotate:  rotate,
		quote: viper.GetViper().GetString(
			"market.quote_currency",
		),
	}
}

/*
Scale sets proposed notional to the capital freed by a displaced incumbent so
Allocator redeploys that lot instead of inventing a max_fraction size.
*/
func (admit *Admit) Scale(
	decision *types.Decision,
	notional *decimal.Decimal,
) {
	if err := admit.validate(map[string]any{
		"decision": decision,
		"notional": notional,
	}); err != nil {
		return
	}

	if notional.Sign() <= 0 {
		errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"notional is zero or negative",
			nil,
		))

		return
	}

	decision.ProposedQuantity = nil
	decision.ProposedNotional = notional.Copy()
}

/*
Reject records a slot-exhausted or rotate-rejected entry as an explicit nothing
decision so the Thesis audit trail shows why utility-ranked intent did not
execute.
*/
func (admit *Admit) Reject(
	thesis *types.Thesis,
	decision types.Decision,
	opportunity bool,
	freeNormal int,
	admittedNormal int,
	incumbents []Incumbent,
) {
	if err := admit.validate(map[string]any{
		"thesis": thesis,
	}); err != nil {
		return
	}

	decision.Action = types.ActionNothing
	prior := decision.Utility
	decision.Utility = 0
	decision.Cause = "slots_full"
	decision.Reason = "higher-utility entries consumed available slots"
	admit.Capital(&decision)

	if decision.Alternatives == nil {
		decision.Alternatives = map[string]float64{}
	}

	if !opportunity && freeNormal <= admittedNormal {
		decision.Reason = "normal slots full; reserved requires opportunity"
	}

	index, found := admit.rotate.Weakest(incumbents)

	if !found {
		thesis.Decisions = append(thesis.Decisions, decision)
		return
	}

	incumbent := incumbents[index]
	edge := prior - incumbent.HoldUtility
	decision.Alternatives["hold_incumbent"] = incumbent.HoldUtility
	decision.Alternatives["exit_cost"] = incumbent.ExitCost
	decision.Alternatives["clear_prob"] = incumbent.ClearProb
	decision.Alternatives["rotate_value"] = edge - incumbent.ExitCost
	decision.Alternatives["wait_value"] = edge * incumbent.ClearProb
	decision.Alternatives["rotate_surplus"] = admit.rotate.Surplus(
		prior, incumbent.HoldUtility, incumbent.ExitCost,
	)

	if !admit.rotate.Gate(
		prior, incumbent.HoldUtility, incumbent.ExitCost, incumbent.ClearProb,
	) {
		decision.Cause = "rotate_wait"
		decision.Reason = "challenger does not clear one-step wait threshold against weakest incumbent"
	}

	thesis.Decisions = append(thesis.Decisions, decision)
}

/*
Accept records lifecycle, decision, and a Thesis holding for Desk to size and
submit. Broker Position construction stays on Desk alone.
*/
func (admit *Admit) Accept(
	thesis *types.Thesis,
	decision types.Decision,
	opportunity bool,
) {
	if admit.quote == "" {
		admit.quote = viper.GetString("market.quote_currency")
	}

	if err := admit.validate(map[string]any{
		"thesis": thesis,
	}); err != nil {
		decision.Action = types.ActionNothing
		decision.Cause = "admit_rejected"
		decision.Reason = err.Error()
		thesis.Decisions = append(thesis.Decisions, decision)
		return
	}

	admit.Capital(&decision)

	if phase, found := thesis.Lifecycle.Load(decision.Symbol); found {
		switch phase {
		case types.LifecycleEntrySelected, types.LifecycleEntrySubmitted,
			types.LifecyclePartiallyEntered, types.LifecycleManaging:
			thesis.Decisions = append(thesis.Decisions, decision)
			return
		}
	}

	thesis.NoteLifecycle(decision.Symbol, types.LifecycleEntrySelected, decision.At)
	thesis.Decisions = append(thesis.Decisions, decision)

	holding := types.NewHolding(admit.ctx, decision.Symbol, decision.ProposedQuantity, decision.ReferencePrice, nil, nil, nil)
	holding.IsOpportunity = opportunity

	// Nil qty means Allocator must size from max_fraction; rotation may leave
	// a positive redeploy quantity already on the decision.
	if decision.ProposedQuantity == nil || decision.ProposedQuantity.Sign() <= 0 {
		holding.Qty = nil
	}

	thesis.Holdings.Store(decision.Symbol, holding)
}

/*
Capital records wallet cash and slot occupancy visible at admit time.
*/
func (admit *Admit) Capital(decision *types.Decision) {
	if err := admit.validate(map[string]any{
		"decision": decision,
	}); err != nil {
		return
	}

	cash, err := admit.balance.FreeCash()

	if err != nil || cash == nil {
		errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"failed to get available cash",
			err,
		))
		return
	}

	decision.AvailableCapital = cash
	decision.SlotCapacity = admit.desk.MaxSlots(false)
	decision.OpenPositions = admit.desk.OpenPositions()
}

func (admit *Admit) validate(mandatory map[string]any) error {
	// context.Background() is a non-nil interface whose concrete value is an
	// empty struct; errnie.Require treats that as missing via IsZero.
	if admit.ctx == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "ctx is required", nil))
	}

	check := map[string]any{
		"balance": admit.balance,
		"desk":    admit.desk,
		"quote":   admit.quote,
	}

	maps.Copy(check, mandatory)

	return errnie.Error(errnie.Require(check))
}
