package broker

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/rawbus"
)

func (desk *Desk) onAction(action *logic.Action) {
	if riskErr := desk.validatePreTrade(action); riskErr != nil {
		desk.recordRiskReject(action, riskErr)
		return
	}

	desk.ensureActionIDs(action)

	clOrdID := uuid.New().String()
	action.ClOrdID = clOrdID

	orderType, orderTypeErr := krakenOrderType(
		action,
		desk.marginEnabled,
		desk.tradingModel,
	)

	if orderTypeErr != nil {
		errnie.Error(orderTypeErr)
		return
	}

	params := trading.AddParams{
		ClOrdID:    clOrdID,
		Symbol:     action.Symbol,
		Side:       action.Side,
		OrderQty:   action.Quantity,
		LimitPrice: action.Price,
		OrderType:  orderType,
	}

	if action.Offset > 0 && isTriggeredOrderType(orderType) {
		params.Triggers = &trading.Triggers{
			Price:     action.Offset,
			PriceType: "pct",
		}
	}

	if !action.Type.IsExit() {
		params.EntryQueuedAt = time.Now().UTC()
	}

	desk.actions.Store(clOrdID, action)

	submitStartedAt := time.Now().UTC()
	sendErr := desk.bus.Send(internal.ChannelKrakenPrivate, "orders", types.KrakenMessage{
		Method: trading.MethodAddOrder,
		Params: params,
		ReqID:  time.Now().UnixNano(),
	})
	submitFinishedAt := time.Now().UTC()

	if errnie.Error(sendErr) != nil {
		desk.actions.Delete(clOrdID)
		desk.markStopNeedsRepair(action)
		desk.recordStopNeedsRepair(action.Symbol, sendErr.Error())
		return
	}

	desk.recordOrderSubmitted(
		action,
		clOrdID,
		submitFinishedAt.Sub(submitStartedAt),
		submitFinishedAt,
	)
	desk.markStopExitSubmitted(action)
}

func (desk *Desk) sendMarketOrder(
	side trading.Side,
	symbol string,
	quantity float64,
) error {
	if symbol == "" || quantity <= 0 {
		return fmt.Errorf(
			"desk: invalid market order symbol=%q quantity=%.8f",
			symbol,
			quantity,
		)
	}

	action := &logic.Action{
		Type:     logic.ActionMarket,
		Side:     side,
		Symbol:   symbol,
		Quantity: quantity,
	}

	// The desk subscribes to its own raw channel: this frame loops back into
	// Tick -> onAction, which stores the action and submits the order exactly
	// once. Submitting here as well doubles every triggered exit.
	return rawbus.Send(desk.bus, rawbus.TypeOrder, action)
}
