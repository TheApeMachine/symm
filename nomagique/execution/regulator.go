package execution

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Fill captures cumulative venue execution facts for an order.
It uses exact Decimal precision for quantity, cost, and fee.
*/
type Fill struct {
	ID            string           `json:"id"`
	OrderID       string           `json:"orderId"`
	ClientOrderID string           `json:"clientOrderId"`
	Side          string           `json:"side"` // "buy" or "sell"
	CumQty        *decimal.Decimal `json:"cumQty"`
	CumCost       *decimal.Decimal `json:"cumCost"`
	Fee           *decimal.Decimal `json:"fee"`
	Status        string           `json:"status"` // "open", "filled", "canceled", "rejected"
}

/*
PositionState represents the current inventory and PnL accounting.
*/
type PositionState struct {
	Symbol   string           `json:"symbol"`
	Quantity *decimal.Decimal `json:"quantity"`
	Basis    *decimal.Decimal `json:"basis"`
	EntryFee *decimal.Decimal `json:"entryFee"`
	Realized *decimal.Decimal `json:"realized"`
}

/*
Regulator is a stateful closure that processes cumulative execution fills
and tracks inventory, basis, and realized PnL with exact Decimal math.
*/
type Regulator types.Value[*Fill, *PositionState]

/*
NewRegulator constructs a stateful position regulator for the given symbol.
*/
func NewRegulator(symbol types.String) Regulator {
	quantity := decimal.NewFromInt64(0)
	basis := decimal.NewFromInt64(0)
	entryFee := decimal.NewFromInt64(0)
	realized := decimal.NewFromInt64(0)

	filled := decimal.NewFromInt64(0)
	cost := decimal.NewFromInt64(0)
	fee := decimal.NewFromInt64(0)

	var lastOrderID string

	return func(report *Fill) *PositionState {
		sym := ""
		if symbol != nil {
			sym = symbol(report)
		}
		if report == nil {
			return &PositionState{
				Symbol:   sym,
				Quantity: quantity,
				Basis:    basis,
				EntryFee: entryFee,
				Realized: realized,
			}
		}

		if report.OrderID != "" && report.OrderID == lastOrderID && report.Status == "filled" {
			return &PositionState{
				Symbol:   sym,
				Quantity: quantity,
				Basis:    basis,
				EntryFee: entryFee,
				Realized: realized,
			}
		}

		cumQty := report.CumQty
		if cumQty == nil {
			cumQty = decimal.NewFromInt64(0)
		}

		cumCost := report.CumCost
		if cumCost == nil {
			cumCost = decimal.NewFromInt64(0)
		}

		cumFee := report.Fee
		if cumFee == nil {
			cumFee = decimal.NewFromInt64(0)
		}

		deltaQty := cumQty.Sub(filled)
		deltaCost := cumCost.Sub(cost)
		deltaFee := cumFee.Sub(fee)

		if deltaQty.Sign() < 0 || deltaCost.Sign() < 0 || deltaFee.Sign() < 0 {
			errnie.Error(errnie.Err(errnie.Validation, "regulator: cumulative execution regressed", nil))
			return nil
		}

		if report.Side == "buy" && deltaQty.Sign() > 0 {
			quantity = quantity.Add(deltaQty)
			basis = basis.Add(deltaCost)
			entryFee = entryFee.Add(deltaFee)
		}

		if report.Side == "sell" && deltaQty.Sign() > 0 {
			if deltaQty.Cmp(quantity) > 0 {
				errnie.Error(errnie.Err(errnie.Validation, "regulator: sale exceeds held inventory", nil))
				return nil
			}

			allocBasis := decimal.NewFromInt64(0)
			allocFee := decimal.NewFromInt64(0)

			if quantity.Sign() > 0 {
				allocBasis = basis.Mul(deltaQty).Div(quantity)
				allocFee = entryFee.Mul(deltaQty).Div(quantity)
			}

			quantity = quantity.Sub(deltaQty)
			basis = basis.Sub(allocBasis)
			entryFee = entryFee.Sub(allocFee)
			realized = realized.Add(deltaCost.Sub(deltaFee).Sub(allocBasis).Sub(allocFee))
		}

		filled = cumQty
		cost = cumCost
		fee = cumFee

		switch report.Status {
		case "filled", "canceled", "expired", "rejected":
			lastOrderID = report.OrderID
			filled = core.ZeroDecimal.Copy()
			cost = core.ZeroDecimal.Copy()
			fee = core.ZeroDecimal.Copy()
		}

		return &PositionState{
			Symbol:   sym,
			Quantity: quantity,
			Basis:    basis,
			EntryFee: entryFee,
			Realized: realized,
		}
	}
}
