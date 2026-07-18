package broker

import (
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

type Position struct {
	status      types.Status
	api         *websocket.API
	instrument  *Instrument
	price       *Price
	balance     *Balance
	pair        *kraken.InstrumentPair
	request     *kraken.MarketOrder
	order       *spot.Order
	orderID     string
	tickers     []*kraken.TickerData
	executions  []*kraken.Execution
	seenExec    sync.Map
	onTerminal  func(symbol string)
	onTicker    func([]byte)
	onOrder     func([]byte)
	onExecution func([]byte)
	subTicker   uint64
	subOrder    uint64
	subExec     uint64
}

/*
NewPosition constructs a position manager and registers its channel handlers.
Subscription ids are retained so Unsubscribe can drop them when the lot closes
instead of leaving orphaned On handlers on the shared ticker channel.
*/
func NewPosition(
	api *websocket.API,
	instrument *Instrument,
	price *Price,
	balance *Balance,
	pair *kraken.InstrumentPair,
) *Position {
	position := &Position{
		status:     types.INITIALIZING,
		api:        api,
		instrument: instrument,
		price:      price,
		balance:    balance,
		pair:       pair,
		tickers:    make([]*kraken.TickerData, 0),
		order: &spot.Order{
			Description: &spot.OrderDescription{
				Pair:      pair.Symbol,
				Type:      "enter",
				OrderType: "market",
			},
			Volume: decimal.NewFromFloat64(0),
			Price:  decimal.NewFromFloat64(0),
		},
	}

	position.onTicker = position.TickerAck
	position.onOrder = position.OrderAck
	position.onExecution = position.ExecutionAck
	position.subscribe()

	return position
}

func (position *Position) Status() types.Status {
	return position.status
}

/*
subscribe registers channel handlers once and stores the ids On returns.
*/
func (position *Position) subscribe() {
	if position == nil || position.api == nil || position.subTicker != 0 {
		return
	}

	position.subTicker = position.api.On("ticker", position.onTicker)
	position.subOrder = position.api.On("add_order", position.onOrder)
	position.subExec = position.api.On("executions", position.onExecution)
}

/*
unsubscribe drops this position's channel handlers so closed lots leave the
ticker path. Safe to call more than once.
*/
func (position *Position) unsubscribe() {
	if position == nil || position.api == nil {
		return
	}

	if position.subTicker != 0 {
		position.api.Unsubscribe("ticker", position.subTicker)
		position.subTicker = 0
	}

	if position.subOrder != 0 {
		position.api.Unsubscribe("add_order", position.subOrder)
		position.subOrder = 0
	}

	if position.subExec != 0 {
		position.api.Unsubscribe("executions", position.subExec)
		position.subExec = 0
	}
}

/*
Close tears down channel handlers when the desk evicts a terminal lot.
*/
func (position *Position) Close() {
	position.balance.holdings.Delete(position.pair.Symbol)
	position.unsubscribe()

	if position.onTerminal != nil && position.pair != nil {
		position.onTerminal(position.pair.Symbol)
	}
}

func (position *Position) OrderAck(buf []byte) {
	orderAck := kraken.NewOrderResponse(buf)

	if position.request == nil || orderAck.ReqID != position.request.ReqID {
		return
	}

	if errnie.Error(kraken.Validate(orderAck)) != nil {
		position.status = types.ERROR
		return
	}

	position.orderID = orderAck.Result.OrderID
	position.status = types.PENDING
}

func (position *Position) ExecutionAck(buf []byte) {
	execution := kraken.NewExecution(buf)

	if errnie.Error(kraken.Validate(execution)) != nil {
		position.status = types.ERROR
		return
	}

	for _, data := range execution.Data {
		if data.OrderID != position.orderID {
			continue
		}

		if data.ExecID != "" {
			if _, seen := position.seenExec.LoadOrStore(data.ExecID, struct{}{}); seen {
				continue
			}
		}

		value, ok := position.balance.holdings.Load(data.Symbol)

		if !ok {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"holding not found for "+data.Symbol,
				nil,
			))

			return
		}

		holding := value.(*types.Holding)
		pnl, err := position.price.WithFriction(position.pair, holding.Qty)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"failed to calculate pnl",
				err,
			))

			return
		}

		holding.Mark = data.LastPrice
		holding.PnL = pnl
		position.executions = append(position.executions, execution)

		status := position.fillStatus(data)
		holding.Status = status
		position.status = status

		if position.balance != nil {
			position.balance.Publish()
		}

		if holding.Status == types.CLOSED {
			position.Close()
		}

		return
	}
}

/*
fillStatus maps an execution print onto inventory status. Buys that trade stay
open; a filled sell closes the lot; cancel prints cancel the pending request.
*/
func (position *Position) fillStatus(data kraken.ExecutionData) types.Status {
	if data.OrderStatus == "canceled" || data.ExecType == "canceled" {
		return types.CANCELED
	}

	if data.Side == "sell" && data.OrderStatus == "filled" {
		return types.CLOSED
	}

	if data.ExecType == "trade" {
		return types.OPEN
	}

	return types.MarketStatuses[data.OrderStatus]
}

func (position *Position) Enter() error {
	holding, err := position.balance.Holding(position.pair.Symbol)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get holding for "+position.pair.Symbol,
			err,
		))
	}

	amount, err := position.price.Taker(position.pair, holding.Qty)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to calculate taker cost: "+err.Error(),
			err,
		))
	}

	if !position.balance.Available(amount) {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"insufficient balance",
			nil,
		))
	}

	position.request = kraken.NewMarketOrder(
		"buy",
		holding.Qty.Float64(),
		holding.Symbol,
	)

	position.status = types.PENDING

	if err := position.api.AddOrder(position.request); err != nil {
		position.status = types.ERROR

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	return nil
}

/*
Exit submits a market sell for the filled position.
*/
func (position *Position) Exit() error {
	holding, err := position.balance.Holding(position.pair.Symbol)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get holding for "+position.pair.Symbol,
			err,
		))
	}

	position.request = kraken.NewMarketOrder(
		"sell",
		holding.Qty.Float64(),
		holding.Symbol,
	)

	position.status = types.PENDING

	if err := position.api.AddOrder(position.request); err != nil {
		position.status = types.OPEN

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	return nil
}

/*
TickerAck routes this symbol's ticker print onto the Holding mark surface and
publishes the desk snapshot. Other symbols on the shared ticker channel are
ignored.
*/
func (position *Position) TickerAck(buf []byte) {
	if position == nil || position.balance == nil || position.pair == nil {
		return
	}

	value, ok := position.balance.holdings.Load(position.pair.Symbol)

	if !ok {
		return
	}

	holding, ok := value.(*types.Holding)

	if !ok || holding == nil {
		return
	}

	ticker := kraken.NewTicker(buf)

	if errnie.Error(kraken.Validate(ticker)) != nil {
		return
	}

	for _, tickerData := range ticker.Data {
		if tickerData.Symbol != position.pair.Symbol {
			continue
		}

		pnl, err := position.price.WithFriction(
			position.pair, holding.Qty,
		)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"failed to calculate pnl",
				err,
			))

			return
		}

		holding.Mark = tickerData.Last
		holding.PnL = pnl
		position.balance.Publish()
		return
	}
}
