package broker

import (
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

type Position struct {
	status     types.Status
	api        *websocket.API
	instrument *Instrument
	price      *Price
	balance    *Balance
	pair       *kraken.InstrumentPair
	request    *kraken.MarketOrder
	orderID    string
	tickers    []*kraken.TickerData
	seenExec   sync.Map
	onTerminal func(symbol string)
}

/*
NewPosition constructs a position manager. Callbacks are registered once on
Desk so closed positions do not accumulate forever on the hot ticker path.
*/
func NewPosition(
	api *websocket.API,
	instrument *Instrument,
	price *Price,
	balance *Balance,
	pair *kraken.InstrumentPair,
) *Position {
	return &Position{
		status:     types.INITIALIZING,
		api:        api,
		instrument: instrument,
		price:      price,
		balance:    balance,
		pair:       pair,
		tickers:    make([]*kraken.TickerData, 0),
	}
}

func (position *Position) Status() types.Status {
	return position.status
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
		holding.Update(&data)
		holding.Mark = &data.LastPrice
		holding.MarkToMarket()
		holding.Executions = append(holding.Executions, execution)
		position.status = holding.Status

		if holding.Closed() || holding.Status == types.CLOSED {
			position.status = types.CLOSED
		}

		return
	}
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
TickerAck updates this position from its own ticker only.

Ticker updates the current mark and recomputes open mark-to-market PnL.
Execution data remains authoritative for entry and exit facts.
*/
func (position *Position) TickerAck(buf []byte) bool {
	holding, err := position.balance.Holding(position.pair.Symbol)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"holding not found for "+position.pair.Symbol,
			nil,
		))

		return false
	}

	ticker := kraken.NewTicker(buf)

	if errnie.Error(kraken.Validate(ticker)) != nil {
		return false
	}

	for _, tickerData := range ticker.Data {
		if tickerData.Symbol != position.pair.Symbol {
			continue
		}

		if tickerData.Last == nil || tickerData.Last.Float64() <= 0 {
			return false
		}

		holding.Mark = tickerData.Last
		holding.MarkToMarket()

		return true
	}

	return false
}
