package broker

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
	LotIncrease owns one outstanding buy and its cumulative fills on an existing

spot lot. Place and Apply run on that lot's guardian, alongside reductions and
exits; no market workspace waits on its venue request.
*/
type LotIncrease struct {
	basis, totalBasis   *decimal.Decimal
	position            *Position
	order               *spot.AddOrderRequest
	orders              []string
	exitID              string
	quantity, cost, fee *decimal.Decimal
}

/* Place validates current economics and commits additional inventory authority. */
func (increase *LotIncrease) Place(decision types.Decision) error {
	position := increase.position

	if (position.status() != types.OPEN && position.status() != types.FILLED) || increase.order != nil || position.ReduceOrder != nil || position.ExitOrder != nil {
		return &types.ExecutionRefusal{State: "no longer executable", Detail: "lot must be open with no pending adjustment"}
	}
	holding := position.Holding

	if holding.EntryVWAP == nil || holding.EntryPrice == nil || holding.EntryFee == nil || holding.EntryQty == nil || holding.EntryFees == nil {
		return &types.ExecutionRefusal{State: "no longer executable", Detail: "increase requires authoritative entry basis"}
	}

	if decision.ProposedQuantity == nil || decision.ProposedQuantity.Sign() <= 0 {
		return &types.ExecutionRefusal{State: "no longer executable", Detail: "positive increase quantity required"}
	}
	cost, err := position.price.EntryCost(position.pair.Symbol, decision.ProposedQuantity)

	if err != nil {
		return &types.ExecutionRefusal{State: "repricing failed", Detail: err.Error()}
	}

	if decision.Admit != nil {
		if err := decision.Admit(cost); err != nil {
			return err
		}
	}

	if decision.Permit != nil && !decision.Permit() {
		return &types.ExecutionRefusal{State: "stale", Detail: "increase authority or candidate changed before submission"}
	}
	increase.basis = Notional(holding.EntryPrice, holding.Qty)
	increase.totalBasis = Notional(holding.EntryVWAP, holding.EntryQty)
	increase.order = &spot.AddOrderRequest{ClOrdId: decision.ID, Type: "buy", OrderType: "market", Volume: decision.ProposedQuantity.String(), Pair: position.pair.Symbol}
	increase.quantity, increase.cost, increase.fee = decimal.NewFromInt64(0), decimal.NewFromInt64(0), decimal.NewFromInt64(0)
	result, err := position.api.AddOrder(increase.order)

	if err != nil {
		increase.order = nil
		return errnie.Err(errnie.IO, "position: increase placement failed", err)
	}
	increase.orders = result.ID
	return nil
}

/*
	Apply incorporates only new cumulative quantity/cost/fees, preserving the

remaining lot's basis through partial fills and repeated execution messages.
*/
func (increase *LotIncrease) Apply(execution kraken.ExecutionData) (bool, error) {
	if increase == nil || increase.order == nil || execution.ClientOrderID != increase.order.ClOrdId {
		return false, nil
	}
	position := increase.position
	status, err := types.StatusFromMarket(execution.OrderStatus)

	if err != nil {
		return true, err
	}
	terminal := status == types.FILLED || status == types.CANCELED || status == types.REJECTED || status == types.EXPIRED

	if terminal && execution.CumQty == nil {
		return true, errnie.Err(errnie.Validation, "position: terminal increase quantity required", nil)
	}

	if execution.CumQty != nil && execution.CumQty.Sign() > 0 {
		if execution.CumCost == nil || execution.FeeUsdEquiv == nil {
			return true, errnie.Err(errnie.Validation, "position: cumulative increase cost and fees required", nil)
		}
		quantity := execution.CumQty.SetScale(max(execution.CumQty.GetScale(), increase.quantity.GetScale())).Sub(increase.quantity)
		cost := execution.CumCost.SetScale(max(execution.CumCost.GetScale(), increase.cost.GetScale())).Sub(increase.cost)
		fee := execution.FeeUsdEquiv.SetScale(max(execution.FeeUsdEquiv.GetScale(), increase.fee.GetScale())).Sub(increase.fee)

		if quantity.Sign() < 0 || cost.Sign() < 0 || fee.Sign() < 0 {
			return true, errnie.Err(errnie.Validation, "position: cumulative increase economics moved backwards", nil)
		}
		holding := position.Holding
		// SDK arithmetic rounds to the receiver scale. Preserve every supplied
		// decimal in sums/products; a nonterminating VWAP uses at least SDK precision.
		precision := max(increase.basis.GetScale(), increase.totalBasis.GetScale(), execution.CumCost.GetScale(), quantity.GetScale())
		basis := increase.basis.SetScale(precision).Add(execution.CumCost)
		holding.Qty = holding.Qty.SetScale(max(holding.Qty.GetScale(), quantity.GetScale())).Add(quantity)
		holding.SellableQty = holding.SellableQty.SetScale(max(holding.SellableQty.GetScale(), quantity.GetScale())).Add(quantity)
		holding.EntryPrice = UnitPrice(basis, holding.Qty).SetScale(max(int64(decimal.DefaultScale), int64(position.pair.PricePrecision)))
		holding.EntryFee = holding.EntryFee.SetScale(max(holding.EntryFee.GetScale(), fee.GetScale())).Add(fee)
		totalBasis := increase.totalBasis.SetScale(precision).Add(execution.CumCost)
		holding.EntryQty = holding.EntryQty.SetScale(max(holding.EntryQty.GetScale(), quantity.GetScale())).Add(quantity)
		holding.EntryVWAP = UnitPrice(totalBasis, holding.EntryQty).SetScale(max(int64(decimal.DefaultScale), int64(position.pair.PricePrecision)))
		holding.EntryFees = holding.EntryFees.SetScale(max(holding.EntryFees.GetScale(), fee.GetScale())).Add(fee)
		increase.quantity, increase.cost, increase.fee = execution.CumQty.Copy(), execution.CumCost.Copy(), execution.FeeUsdEquiv.Copy()

		if position.recordFill != nil {
			position.recordFill("increase_fill", execution)
		}

		if position.store != nil {
			if err := position.store.Save(holding); err != nil {
				return true, err
			}
		}
	}

	if terminal {
		if position.recordFill != nil {
			position.recordFill("execution_terminal", execution)
		}
		increase.order = nil

		if increase.exitID != "" {
			identity := increase.exitID
			increase.exitID = ""
			err := position.executeManualExit(identity)
			kind := "execution_submitted"

			if err != nil {
				kind = "execution_failed"
			}

			if position.recordFill != nil {
				position.recordFill(kind, kraken.ExecutionData{ClientOrderID: identity, Timestamp: execution.Timestamp})
			}

			return true, err
		}
	}
	return true, nil
}

/*
	Cancel preserves an exit request while outstanding buys are cancelled and

reconciled. The terminal buy fact determines the full inventory to liquidate;
closing the lot before that fact could abandon a late partial buy.
*/
func (increase *LotIncrease) Cancel(identity string) error {
	if increase.exitID != "" {
		return &types.ExecutionRefusal{State: "reduction pending", Detail: "exit is already awaiting terminal buy reconciliation"}
	}

	if len(increase.orders) == 0 {
		return errnie.Err(errnie.Validation, "position: venue order identity required to cancel increase", nil)
	}
	increase.exitID = identity
	for _, order := range increase.orders {
		if _, err := increase.position.api.CancelOrder(&spot.CancelOrderRequest{TxID: order}); err != nil {
			increase.exitID = ""
			return errnie.Err(errnie.IO, "position: cancel increase before liquidation", err)
		}
	}
	return nil
}
