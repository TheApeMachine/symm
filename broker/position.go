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
	ui chan []byte,
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
		position.status = types.REJECTED

		if holding, err := position.balance.Holding(position.Stop.Symbol); err == nil &&
			holding.Qty.Sign() > 0 {
			position.status = types.OPEN
		}

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
		if position.orderID == "" || data.OrderID != position.orderID || data.Symbol != position.pair.Symbol {
			continue
		}

		found, _ := position.balance.holdings.LoadOrStore(position.pair.Symbol, types.Holding{
			Status:     types.MarketStatuses[data.ExecType],
			Symbol:     position.pair.Symbol,
			Asset:      position.pair.Base,
			Qty:        decimal.NewFromFloat64(data.LastQty),
			EntryAt:    &data.Timestamp,
			EntryPrice: &data.LastPrice,
			EntryFee:   &data.FeeUsdEquiv,
			ExitPrice:  &data.LastPrice,
			ExitFee:    &data.FeeUsdEquiv,
		})

		pair, err := position.instrument.Pair(position.pair.Symbol)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"failed to get price for "+position.pair.Symbol,
				err,
			))

			return
		}

		holding := found.(types.Holding)
		holding.Mark = &data.LastPrice
		holding.PnL, err = position.price.WithFriction(&pair, holding.Qty)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"failed to calculate friction for "+position.pair.Symbol,
				err,
			))

			return
		}

		return
	}
}

func (position *Position) Enter() error {
	holding, err := position.balance.Holding(position.Stop.Symbol)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get holding for "+position.Stop.Symbol,
			err,
		))
	}

	instrument, err := position.instrument.Pair(position.Stop.Symbol)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get instrument for "+position.Stop.Symbol,
			err,
		))
	}

	if holding.Qty.Cmp(decimal.NewFromFloat64(instrument.QtyMin)) < 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"position quantity is less than minimum quantity",
			nil,
		))
	}

	amount, err := position.price.Taker(&instrument, holding.Qty)

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

	if holding.Qty.Sign() <= 0 || holding.Qty.Cmp(holding.Qty) > 0 {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get holding for "+position.Stop.Symbol,
			err,
		))
	}

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

func (position *Position) Executions() []*kraken.Execution {
	if position.Stop == nil {
		return nil
	}

	holding, err := position.balance.Holding(position.Stop.Symbol)

	if err != nil {
		return nil
	}

	return holding.Executions
}

/*
TickerAck updates this position from its own ticker only.

Position does not perform fee, notional, PnL, or return calculations.
It delegates the entire valuation to Price.
*/
func (position *Position) TickerAck(buf []byte) {
	holding, err := position.balance.Holding(position.Stop.Symbol)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get holding for "+position.Stop.Symbol,
			err,
		))

		return
	}

	ticker := kraken.NewTicker(buf)

	if errnie.Error(kraken.Validate(ticker)) != nil {
		return
	}

	for _, tickerData := range ticker.Data {
		if tickerData.Symbol != position.Stop.Symbol ||
			tickerData.Last == nil ||
			holding.Order.Price.Sign() <= 0 ||
			holding.Order.Volume.Sign() <= 0 {
			continue
		}

		holding.Mark = tickerData.Last
		holding.PnL, err = position.price.WithFriction(position.pair, holding.Qty)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"failed to calculate friction for "+position.Stop.Symbol,
				err,
			))

			return
		}

		return
	}
}
