package broker

import (
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
OrderAck binds the venue order id for the in-flight request and drains buffered
executions that arrived before the ack.
*/
func (position *Position) OrderAck(ack *kraken.OrderResponse) {
	if position.request == nil || ack.ReqID != position.request.ReqID {
		return
	}

	if errnie.Error(kraken.Validate(ack)) != nil {
		position.request = nil
		_ = position.setStatus(types.ERROR)

		return
	}

	position.orderID = ack.Result.OrderID
	position.drainBuffered()

	if position.status == types.OPEN ||
		position.status == types.CLOSED ||
		position.status == types.CANCELED ||
		position.status == types.FILLED {
		return
	}

	_ = position.setStatus(types.PENDING)
}

/*
ExecutionAck decodes one raw execution frame for tests and direct callers.
Desk routes through ApplyExecution after a single decode.
*/
func (position *Position) ExecutionAck(buf []byte) {
	execution := kraken.NewExecution(buf)

	if errnie.Error(kraken.Validate(execution)) != nil {
		return
	}

	for _, data := range execution.Data {
		position.ApplyExecution(data)
	}
}

/*
OrderAckRaw decodes one add_order acknowledgement for tests.
*/
func (position *Position) OrderAckRaw(buf []byte) {
	position.OrderAck(kraken.NewOrderResponse(buf))
}

/*
ApplyExecution routes one already-decoded execution row onto this lot.
*/
func (position *Position) ApplyExecution(data kraken.ExecutionData) {
	if data.ExecID != "" {
		if _, seen := position.seenExec[data.ExecID]; seen {
			return
		}
	}

	if !position.accept(data.OrderID) {
		position.buffered = append(position.buffered, data)

		return
	}

	position.applyFill(data)
}

func (position *Position) drainBuffered() {
	if len(position.buffered) == 0 {
		return
	}

	pending := position.buffered
	position.buffered = nil

	for _, data := range pending {
		if data.OrderID != "" && data.OrderID != position.orderID {
			position.buffered = append(position.buffered, data)
			continue
		}

		position.applyFill(data)
	}
}

func (position *Position) applyFill(data kraken.ExecutionData) {
	symbol := position.holdingSymbol(data.Symbol)

	if symbol == "" {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"position: nil holding for execution symbol "+data.Symbol,
			nil,
		))

		return
	}

	if data.ExecID != "" {
		position.seenExec[data.ExecID] = struct{}{}
	}

	data.Side = strings.ToLower(data.Side)
	closed := false

	if err := position.balance.Update(symbol, func(holding *types.Holding) error {
		_ = position.price.RecordFill(&position.pair, holding, data, &position.fills)

		status, err := types.StatusFromMarket(data.ExecType)

		if err != nil {
			return err
		}

		if holding.Qty == nil || holding.Qty.Sign() <= 0 {
			status = types.CLOSED
		}

		if data.Side == "sell" && data.OrderStatus == "filled" {
			status = types.CLOSED
		}

		if holdErr := position.transitionHolding(holding, status); holdErr != nil {
			return holdErr
		}

		closed = holding.Status == types.CLOSED || holding.Status == types.CANCELED

		return position.setStatus(status)
	}); err != nil {
		errnie.Error(err)

		return
	}

	if position.intentID != "" && position.balance != nil {
		_ = position.balance.Commit(position.intentID)
		position.intentID = ""
	}

	if err := position.balance.Publish(); err != nil {
		errnie.Error(err)
	}

	if closed {
		position.Close()
	}
}

func (position *Position) transitionHolding(
	holding *types.Holding,
	status types.Status,
) error {
	next, err := types.Transition(holding.Status, status)

	if err != nil {
		return errnie.Error(err)
	}

	holding.Status = next

	return nil
}

/*
accept reports whether this execution belongs to the position.
*/
func (position *Position) accept(orderID string) bool {
	if position.orderID != "" {
		return orderID == position.orderID
	}

	if position.request == nil || orderID == "" {
		return false
	}

	position.orderID = orderID

	return true
}

/*
holdingSymbol resolves which inventory key owns this execution symbol.
*/
func (position *Position) holdingSymbol(symbol string) string {
	for _, key := range []string{
		position.pair.Symbol,
		symbol,
		position.api.Name(symbol),
	} {
		if key == "" {
			continue
		}

		if _, err := position.balance.Holding(key); err == nil {
			return key
		}
	}

	return ""
}

/*
Enter seeds the holding onto Balance and submits a market buy for its quantity.
*/
func (position *Position) Enter(holding *types.Holding) error {
	if holding.Asset == "" {
		holding.Asset = position.pair.Base
	}

	position.balance.StoreHolding(holding)

	amount, err := position.price.Taker(&position.pair, holding.Qty)

	if err != nil {
		position.balance.DeleteHolding(holding.Symbol)

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to calculate taker cost: "+err.Error(),
			err,
		))
	}

	// Desk already reserved cash under intentID; Available would subtract it twice.
	if position.intentID == "" {
		ok, err := position.balance.Available(amount)

		if err != nil {
			position.balance.DeleteHolding(holding.Symbol)

			return errnie.Error(err)
		}

		if !ok {
			position.balance.DeleteHolding(holding.Symbol)

			return errnie.Error(errnie.Err(
				errnie.Internal,
				"insufficient balance",
				nil,
			))
		}
	}

	position.request = kraken.NewMarketOrder(
		"buy",
		holding.Qty,
		holding.Symbol,
	)

	if err := position.setStatus(types.PENDING); err != nil {
		position.balance.DeleteHolding(holding.Symbol)
		position.request = nil

		return errnie.Error(err)
	}

	if err := position.api.AddOrder(position.request); err != nil {
		position.balance.DeleteHolding(holding.Symbol)
		position.request = nil
		_ = position.setStatus(types.ERROR)

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	return nil
}

/*
Exit submits a market sell for the sellable ledger quantity.
*/
func (position *Position) Exit() error {
	if position.status == types.PENDING {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"position is pending",
			nil,
		))
	}

	holding, err := position.balance.Holding(position.pair.Symbol)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"failed to get holding for "+position.pair.Symbol,
			err,
		))
	}

	asset := holding.Asset

	if asset == "" {
		asset = position.pair.Base
	}

	available, err := position.balance.AssetAvailable(asset)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"no wallet availability to sell "+position.pair.Symbol,
			err,
		))
	}

	if available.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"no sellable "+asset+" available for "+position.pair.Symbol,
			nil,
		))
	}

	quantity := position.price.Quantize(&position.pair, available)

	if quantity == nil || quantity.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"exit quantity not executable for "+position.pair.Symbol,
			nil,
		))
	}

	if holding.Qty != nil && quantity.Cmp(holding.Qty) > 0 {
		quantity = position.price.Quantize(&position.pair, holding.Qty)
	}

	intentID := "exit:" + position.pair.Symbol + ":" + quantity.String()

	if err := position.balance.ReserveAsset(
		intentID, asset, quantity,
	); err != nil {
		return err
	}

	position.intentID = intentID
	position.request = kraken.NewMarketOrder(
		"sell",
		quantity,
		holding.Symbol,
	)

	prior := position.status

	if err := position.setStatus(types.PENDING); err != nil {
		_ = position.balance.Release(intentID)
		position.intentID = ""
		position.request = nil

		return errnie.Error(err)
	}

	if err := position.api.AddOrder(position.request); err != nil {
		_ = position.balance.Release(intentID)
		position.intentID = ""
		position.request = nil
		_ = position.setStatus(prior)

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market sell order",
			err,
		))
	}

	return nil
}

/*
MatchesReq reports whether an order ack belongs to this lot.
*/
func (position *Position) MatchesReq(reqID int64) bool {
	return position.request != nil && position.request.ReqID == reqID
}

/*
MatchesOrder reports whether an execution order id belongs to this lot.
*/
func (position *Position) MatchesOrder(orderID string) bool {
	if orderID == "" {
		return false
	}

	if position.orderID != "" {
		return position.orderID == orderID
	}

	return position.request != nil
}

/*
Symbol returns the instrument symbol for this lot.
*/
