package broker

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func (desk *Desk) onExecutions(message any) any {
	execution := kraken.NewExecution(message.([]byte))

	if errnie.Error(kraken.Validate(execution)) != nil {
		return nil
	}

	for _, data := range execution.Data {
		position := desk.byOrderID[data.OrderID]

		if position == nil {
			for _, candidate := range desk.positions {
				if candidate.MatchesOrder(data.OrderID) {
					position = candidate
					desk.byOrderID[data.OrderID] = candidate
					break
				}
			}
		}

		if position == nil {
			continue
		}

		position.ApplyExecution(data)

		if position.orderID != "" {
			desk.byOrderID[position.orderID] = position
		}
	}

	return nil
}

/*
onOrder decodes one add_order ack and routes by request-id index.
*/
func (desk *Desk) onOrder(message any) any {
	ack := kraken.NewOrderResponse(message.([]byte))
	position := desk.byReqID[ack.ReqID]

	if position == nil {
		for _, candidate := range desk.positions {
			if candidate.MatchesReq(ack.ReqID) {
				position = candidate
				desk.byReqID[ack.ReqID] = candidate
				break
			}
		}
	}

	if position == nil {
		return nil
	}

	position.OrderAck(ack)

	if position.orderID != "" {
		desk.byOrderID[position.orderID] = position
	}

	return nil
}


func (desk *Desk) Buy(
	holding *types.Holding,
	opportunity bool,
) (*Position, error) {
	return desk.ReserveAndSubmitEntry(holding, opportunity)
}

/*
ReserveAndSubmitEntry claims slot+cash then submits the market enter as one
Desk transition so a failed submit releases both reservations.
*/
func (desk *Desk) ReserveAndSubmitEntry(
	holding *types.Holding,
	opportunity bool,
) (*Position, error) {
	if holding.Qty == nil || holding.Qty.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "desk: enter requires positive quantity", nil,
		))
	}

	if !desk.HasSlot(opportunity) {
		return nil, errnie.Error(errnie.Err(
			errnie.Conflict, "desk: no free slot for "+holding.Symbol, nil,
		))
	}

	pair, err := desk.instrument.Pair(holding.Symbol)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"desk: instrument pair unavailable for "+holding.Symbol,
			err,
		))
	}

	mark, markErr := desk.price.Last(holding.Symbol)

	if markErr != nil || mark == nil || mark.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"desk: mark unavailable for reservation on "+holding.Symbol,
			markErr,
		))
	}

	intentID := holding.Symbol + ":" + holding.Qty.String()
	cash := holding.Qty.Mul(mark)

	if _, exists := desk.balance.Ledger().byID[intentID]; !exists {
		if err := desk.balance.Ledger().Reserve(
			intentID, holding.Symbol, cash, true,
		); err != nil {
			return nil, err
		}
	}

	position := NewPosition(
		desk.api, desk.instrument, desk.price, desk.balance, pair,
	)
	position.intentID = intentID

	if err := position.Enter(holding); err != nil {
		_ = desk.balance.Ledger().Release(intentID)

		return nil, err
	}

	// Venue now owns the working order; drop the local claim so slot math and
	// Available track exchange state plus any still-open Allocator claims.
	_ = desk.balance.Ledger().Commit(intentID)
	position.intentID = ""

	if position.request != nil {
		desk.byReqID[position.request.ReqID] = position
	}

	desk.positions[holding.Symbol] = position

	return position, nil
}

/*
Sell exits the full desk-owned sellable lot for symbol.
*/
func (desk *Desk) Sell(symbol string) error {
	position, ok := desk.Position(symbol)

	if !ok {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"desk: no open position for "+symbol,
			nil,
		))
	}

	if err := position.Exit(); err != nil {
		return err
	}

	if position.request != nil {
		desk.byReqID[position.request.ReqID] = position
	}

	return nil
}

