package broker

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

type StopData struct {
	Symbol     string          `json:"symbol"`
	Armed      bool            `json:"-"`
	PeakPrice  decimal.Decimal `json:"-"`
	StopPrice  decimal.Decimal `json:"stop_price"`
	PeakReturn float64         `json:"peak_return"`
	StopReturn float64         `json:"stop_return"`
}

type Position struct {
	status     types.Status
	api        *websocket.API
	instrument *Instrument
	price      *Price
	balance    *Balance
	pair       *kraken.InstrumentPair
	request    *kraken.MarketOrder
	orderID    string
	Stop       *StopData
	tickers    []*kraken.TickerData
}

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
		Stop:       &StopData{Symbol: pair.Symbol},
	}

	position.api.On("add_order", position.OrderAck)
	position.api.On("executions", position.ExecutionAck)
	position.api.On("ticker", position.TickerAck)

	return position
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
		holding.Executions = append(holding.Executions, execution)

		if data.Side == "sell" && data.ExecType == "trade" {
			holding.Status = types.CLOSED
			position.status = types.CLOSED
			return
		}

		position.status = holding.Status
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

	if holding.Qty.Cmp(decimal.NewFromFloat64(position.pair.QtyMin)) < 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"position quantity is less than minimum quantity",
			nil,
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

	if !position.balance.Available(*amount) {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"insufficient balance",
			nil,
		))
	}

	request := kraken.NewMarketOrder(
		"buy",
		holding.Qty.Float64(),
		holding.Symbol,
	)

	position.request = request
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

func (position *Position) Exit() error {
	holding, err := position.balance.Holding(position.Stop.Symbol)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get holding for "+position.Stop.Symbol,
			err,
		))
	}

	request := kraken.NewMarketOrder(
		"sell",
		holding.Qty.Float64(),
		holding.Symbol,
	)

	position.request = request
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

Ticker updates only the current mark. Execution data remains authoritative for
entry and exit facts, and no mark-to-market value is invented here.
*/
func (position *Position) TickerAck(buf []byte) {
	value, ok := position.balance.holdings.Load(position.Stop.Symbol)

	if !ok {
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"holding not found for "+position.Stop.Symbol,
			nil,
		))

		return
	}

	ticker := kraken.NewTicker(buf)

	if errnie.Error(kraken.Validate(ticker)) != nil {
		return
	}

	for _, tickerData := range ticker.Data {
		if tickerData.Symbol != position.Stop.Symbol {
			continue
		}

		value.(*types.Holding).Mark = tickerData.Last
		return
	}
}
