package broker

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

var orderStatuses = map[string]types.Status{
	"open":              types.OPEN,
	"filled":            types.FILLED,
	"cancelled":         types.CANCELLED,
	"rejected":          types.REJECTED,
	"expired":           types.EXPIRED,
	"partial":           types.PARTIAL,
	"partial_filled":    types.PARTIAL_FILLED,
	"partially_filled":  types.PARTIAL_FILLED,
	"partial_cancelled": types.PARTIAL_CANCELLED,
	"partial_rejected":  types.PARTIAL_REJECTED,
	"partial_expired":   types.PARTIAL_EXPIRED,
}

type PositionData struct {
	Symbol     string          `json:"symbol"`
	Qty        decimal.Decimal `json:"qty"`
	EntryPrice decimal.Decimal `json:"entry_price"`
	Mark       decimal.Decimal `json:"mark"`
	PnL        decimal.Decimal `json:"pnl"`
	ReturnPct  float64         `json:"return_pct"`
}

type StopData struct {
	Symbol     string          `json:"symbol"`
	Armed      bool            `json:"-"`
	PeakPrice  decimal.Decimal `json:"-"`
	StopPrice  decimal.Decimal `json:"stop_price"`
	PeakReturn float64         `json:"peak_return"`
	StopReturn float64         `json:"stop_return"`
}

type Position struct {
	status       types.Status
	api          *websocket.API
	ui           chan []byte
	price        *Price
	balance      *Balance
	orderID      string
	clientID     int
	reqID        int
	requestedQty decimal.Decimal
	order        *kraken.LimitOrder
	executions   []*kraken.Execution
	Data         *PositionData
	Stop         *StopData
	tickers      []*kraken.TickerData
}

func NewPosition(
	api *websocket.API,
	ui chan []byte,
	price *Price,
	balance *Balance,
	data *PositionData,
) *Position {
	position := &Position{
		status:     types.INITIALIZING,
		api:        api,
		ui:         ui,
		price:      price,
		balance:    balance,
		Data:       data,
		executions: make([]*kraken.Execution, 0),
		tickers:    make([]*kraken.TickerData, 0),
	}

	position.api.On("add_order", position.OrderAck)
	position.api.On("executions", position.ExecutionAck)
	position.api.On("ticker", position.TickerAck)

	return position
}

func (position *Position) Status() types.Status {
	return position.status
}

func (position *Position) Publish() {
	position.ui <- datura.Map[any]{
		"positions": []PositionData{
			*position.Data,
		},
	}.Marshal()
}

/*
Hydrate connects a position to an existing wallet holding and its
corresponding buy trade.

Price owns the valuation calculation. Position only stores the result.
*/
func (position *Position) Hydrate(
	symbol string,
	history *kraken.TradesHistory,
) *Position {
	if errnie.Error(kraken.Validate(history)) != nil {
		return position
	}

	if position.balance == nil ||
		position.api == nil ||
		position.price == nil {
		return position
	}

	position.Data.Symbol = symbol

	for _, holding := range position.balance.Holdings() {
		if holding.Asset == position.balance.quote || holding.Qty.Sign() <= 0 {
			continue
		}

		for _, trade := range history.Result.Trades {
			if trade.Pair != symbol || trade.Type != "buy" || trade.Price == nil {
				continue
			}

			position.Data.Qty = holding.Qty
			position.Data.EntryPrice = *trade.Price

			quote, err := position.price.PositionQuote(
				position.Data.Symbol,
				position.Data.EntryPrice,
				position.Data.Qty,
			)

			if err != nil {
				errnie.Warn(err.Error())
				return position
			}

			position.Data.Mark = quote.Mark
			position.Data.PnL = quote.PnL
			position.Data.ReturnPct = quote.ReturnPct

			position.status = types.OPEN
			position.Publish()

			return position
		}
	}

	return position
}

func (position *Position) OrderAck(buf []byte) {
	orderAck := kraken.NewOrderResponse(buf)

	if errnie.Error(kraken.Validate(orderAck)) != nil {
		position.status = types.ERROR
		return
	}

	if orderAck.ReqID != position.reqID {
		return
	}

	position.orderID = orderAck.Result.OrderID
	position.clientID = orderAck.Result.OrderUserref
	position.status = types.OPEN

	position.Publish()
}

func (position *Position) ExecutionAck(buf []byte) {
	execution := kraken.NewExecution(buf)

	if errnie.Error(kraken.Validate(execution)) != nil {
		position.status = types.ERROR
		return
	}

	matched := false

	for _, executionData := range execution.Data {
		if executionData.OrderID != position.orderID {
			continue
		}

		cumulativeQuantity := decimal.NewFromFloat64(
			executionData.CumQty,
		)

		switch executionData.Side {
		case "buy":
			position.Data.Qty = *cumulativeQuantity
			position.Data.EntryPrice = executionData.AvgPrice

		case "sell":
			position.Data.Qty = *position.requestedQty.Sub(
				cumulativeQuantity,
			)
		}

		position.Data.Mark = executionData.LastPrice

		if status, ok := orderStatuses[executionData.OrderStatus]; ok {
			position.status = status
		}

		matched = true
	}

	if matched {
		if err := position.Execution(execution); err != nil {
			position.status = types.ERROR
			return
		}

		position.executions = append(
			position.executions,
			execution,
		)

		//Refresh the PnL after the execution changes quantity
		//or average entry price.
		if position.Data.Qty.Sign() > 0 &&
			position.Data.EntryPrice.Sign() > 0 {
			quote, err := position.price.PositionQuote(
				position.Data.Symbol,
				position.Data.EntryPrice,
				position.Data.Qty,
			)

			if err == nil {
				position.Data.Mark = quote.Mark
				position.Data.PnL = quote.PnL
				position.Data.ReturnPct = quote.ReturnPct
			} else {
				errnie.Error(err)
			}
		}
	}

	position.Publish()
}

/*
Execution validates an execution belonging to this position.
Fee and PnL calculations do not belong here. Price owns those
calculations centrally.
*/
func (position *Position) Execution(
	execution *kraken.Execution,
) error {
	if errnie.Error(kraken.Validate(execution)) != nil {
		position.status = types.ERROR

		return errnie.Error(errnie.Err(
			errnie.Validation,
			"invalid execution",
			nil,
		))
	}

	return nil
}

func (position *Position) Enter() error {
	position.requestedQty = position.Data.Qty

	/*
		Taker returns the estimated quote-currency cost of buying
		the requested quantity, including one taker fee.
	*/
	amount, err := position.price.Taker(
		position.Data.Symbol,
		position.requestedQty,
	)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get taker: "+err.Error(),
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

	order := kraken.NewMarketOrder(
		"buy",
		position.requestedQty.Float64(),
		position.Data.Symbol,
	)

	position.reqID = order.ReqID
	position.status = types.PENDING

	if err := position.api.AddOrder(order); err != nil {
		position.status = types.ERROR

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	position.Publish()

	return nil
}

func (position *Position) Exit() error {
	if position.Data.Qty.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Forbidden,
			"position has no quantity to sell",
			nil,
		))
	}

	position.requestedQty = position.Data.Qty

	order := kraken.NewMarketOrder(
		"sell",
		position.requestedQty.Float64(),
		position.Data.Symbol,
	)

	position.reqID = order.ReqID
	position.status = types.PENDING

	if err := position.api.AddOrder(order); err != nil {
		position.status = types.ERROR

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	position.Publish()

	return nil
}

func (position *Position) Executions() []*kraken.Execution {
	return position.executions
}

/*
TickerAck updates this position from its own ticker only.

Position does not perform fee, notional, PnL, or return calculations.
It delegates the entire valuation to Price.
*/
func (position *Position) TickerAck(buf []byte) {
	ticker := kraken.NewTicker(buf)

	if errnie.Error(kraken.Validate(ticker)) != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"invalid ticker",
			nil,
		))

		return
	}

	for _, tickerData := range ticker.Data {
		if tickerData.Symbol != position.Data.Symbol ||
			tickerData.Last == nil ||
			position.Data.EntryPrice.Sign() <= 0 ||
			position.Data.Qty.Sign() <= 0 {
			continue
		}

		quote, err := position.price.PositionQuoteAt(
			position.Data.Symbol,
			position.Data.EntryPrice,
			*tickerData.Last,
			position.Data.Qty,
		)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"failed to get position quote",
				err,
			))

			return
		}

		position.Data.Mark = quote.Mark
		position.Data.PnL = quote.PnL
		position.Data.ReturnPct = quote.ReturnPct

		position.Publish()

		return
	}
}
