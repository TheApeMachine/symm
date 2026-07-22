package broker

import (
	"context"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Position owns one venue order lifecycle and its exact transport subscriptions.
It releases those subscriptions when the lot closes so completed trades do not
remain on future ticker and execution hot paths.
*/
type Position struct {
	mu      sync.RWMutex
	status  types.Status
	api     *websocket.API
	price   *Price
	balance *Balance
	pair    *kraken.InstrumentPair
	request *kraken.MarketOrder
	orderID string
	ctx     context.Context
	cancel  context.CancelFunc
}

/*
NewPosition registers order, execution, and ticker handlers for one lot.
*/
func NewPosition(
	api *websocket.API,
	price *Price,
	balance *Balance,
	pair *kraken.InstrumentPair,
) *Position {
	ctx, cancel := context.WithCancel(api.Context())

	position := &Position{
		status:  types.INITIALIZING,
		api:     api,
		price:   price,
		balance: balance,
		pair:    pair,
		ctx:     ctx,
		cancel:  cancel,
	}

	go position.consumeOrders()
	go position.consumeExecutions()
	go position.consumeTickers()

	return position
}

/*
consumeOrders ranges the typed order-acknowledgement stream until the position
context is cancelled.
*/
func (position *Position) consumeOrders() {
	channel := position.api.OrderChannel()

	for {
		select {
		case <-position.ctx.Done():
			return
		case ack := <-channel:
			position.OrderAck(ack)
		}
	}
}

/*
consumeExecutions ranges the typed executions stream until the position context
is cancelled.
*/
func (position *Position) consumeExecutions() {
	channel := position.api.ExecutionsChannel()

	for {
		select {
		case <-position.ctx.Done():
			return
		case execution := <-channel:
			position.ExecutionAck(execution)
		}
	}
}

/*
consumeTickers ranges the typed ticker stream until the position context is
cancelled.
*/
func (position *Position) consumeTickers() {
	channel := position.api.TickerChannel()

	for {
		select {
		case <-position.ctx.Done():
			return
		case data := <-channel:
			position.TickerAck(data)
		}
	}
}

/*
Status reports the latest order or holding lifecycle observed from the venue.
*/
func (position *Position) Status() types.Status {
	position.mu.RLock()
	defer position.mu.RUnlock()

	return position.status
}

/*
Close stops the order, execution, and ticker consumer goroutines by cancelling
the position context.
*/
func (position *Position) Close() {
	if position.cancel != nil {
		position.cancel()
	}
}

/*
TickerAck marks the open holding from this symbol's ticker print and feeds its
Stoploss.
*/
func (position *Position) TickerAck(data []kraken.TickerData) {
	for index := range data {
		if data[index].Symbol != position.pair.Symbol {
			continue
		}

		value, ok := position.balance.holdings.Load(position.pair.Symbol)

		if !ok {
			return
		}

		holding := value.(*types.Holding)

		if holding.Stoploss == nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"holding has no stoploss for "+position.pair.Symbol,
				nil,
			))

			return
		}

		holding.Stoploss.Lock()

		if holding.Status != types.OPEN {
			holding.Stoploss.Unlock()
			return
		}

		_ = position.price.Mark(position.pair, holding)

		if holding.StopMark != nil {
			holding.Stoploss.ObserveMark(holding.StopMark.Float64())
		}

		holding.Stoploss.Unlock()

		position.balance.Publish()

		return
	}
}

/*
OrderAck binds the matching venue order identifier to this pending position.
*/
func (position *Position) OrderAck(orderAck *kraken.OrderResponse) {
	position.mu.Lock()
	defer position.mu.Unlock()

	if position.request == nil || orderAck.ReqID != position.request.ReqID {
		return
	}

	if errnie.Error(kraken.Validate(orderAck)) != nil {
		position.request = nil
		position.status = types.ERROR
		return
	}

	position.orderID = orderAck.Result.OrderID
	position.status = types.PENDING
}

/*
ExecutionAck applies matching fills to wallet inventory and releases all
subscriptions once the lot reaches a terminal state.
*/
func (position *Position) ExecutionAck(execution *kraken.Execution) {
	if errnie.Error(kraken.Validate(execution)) != nil {
		return
	}

	for _, data := range execution.Data {
		position.mu.RLock()
		orderID := position.orderID
		position.mu.RUnlock()

		if data.OrderID != orderID {
			continue
		}

		value, ok := position.balance.holdings.Load(data.Symbol)

		if !ok {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"holding not found for "+data.Symbol,
				nil,
			))

			continue
		}

		holding := value.(*types.Holding)

		if holding.Stoploss == nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"holding has no stoploss for "+data.Symbol,
				nil,
			))

			continue
		}

		holding.Stoploss.Lock()
		position.price.Fill(position.pair, holding, data)

		status, ok := types.MarketStatuses[data.ExecType]

		if !ok {
			status = types.Status(data.ExecType)
		}

		if holding.Qty == nil || holding.Qty.Sign() <= 0 {
			status = types.CLOSED
		}

		holding.Status = status
		holding.Stoploss.Unlock()
		position.mu.Lock()
		position.status = status
		position.mu.Unlock()
		position.balance.Publish()

		if status == types.CLOSED || status == types.CANCELED {
			position.Close()
		}
	}
}

/*
Enter seeds the holding onto Balance and submits a market buy for its quantity.
*/
func (position *Position) Enter(holding *types.Holding) error {
	if holding == nil || holding.Stoploss == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"holding and stoploss are required for entry",
			nil,
		))
	}

	if holding.Asset == "" {
		holding.Asset = position.pair.Base
	}

	position.balance.holdings.Store(holding.Symbol, holding)

	amount, err := position.price.Taker(position.pair, holding.Qty)

	if err != nil {
		position.balance.holdings.Delete(holding.Symbol)

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to calculate taker cost: "+err.Error(),
			err,
		))
	}

	if !position.balance.Available(amount) {
		position.balance.holdings.Delete(holding.Symbol)

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"insufficient balance",
			nil,
		))
	}

	request := kraken.NewMarketOrder(
		"buy",
		holding.Qty,
		holding.Symbol,
	)

	position.mu.Lock()
	position.request = request
	position.status = types.PENDING
	position.mu.Unlock()

	if err := position.api.AddOrder(request); err != nil {
		position.balance.holdings.Delete(holding.Symbol)
		position.mu.Lock()
		position.status = types.ERROR
		position.mu.Unlock()

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	return nil
}

/*
Exit submits a market sell for the full filled quantity.
*/
func (position *Position) Exit() error {
	if position.Status() == types.PENDING {
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

	if holding.Qty == nil || holding.Qty.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"quantity must be positive for "+position.pair.Symbol,
			nil,
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

	request := kraken.NewMarketOrder(
		"sell",
		holding.Qty,
		holding.Symbol,
	)

	position.mu.Lock()
	position.request = request
	position.status = types.PENDING
	position.mu.Unlock()

	if err := position.api.AddOrder(request); err != nil {
		position.mu.Lock()
		position.status = types.ERROR
		position.mu.Unlock()

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market sell order",
			err,
		))
	}

	return nil
}
