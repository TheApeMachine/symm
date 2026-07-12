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
	Symbol     string           `json:"symbol"`
	Qty        decimal.Decimal  `json:"qty"`
	EntryPrice decimal.Decimal  `json:"entry_price"`
	Mark       decimal.Decimal  `json:"mark"`
	PnL        decimal.Decimal  `json:"pnl"`
	ReturnPct  float64          `json:"return_pct"`
	FeeRate    *decimal.Decimal `json:"fee_rate,omitempty"`
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
	valuation    *Valuation
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
		valuation:  &Valuation{},
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
		"positions": []PositionData{*position.Data},
	}.Marshal()
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

		cumulativeQuantity := decimal.NewFromFloat64(executionData.CumQty)

		if executionData.Side == "buy" {
			position.Data.Qty = *cumulativeQuantity
			position.Data.EntryPrice = executionData.AvgPrice
		}

		if executionData.Side == "sell" {
			position.Data.Qty = *position.requestedQty.Sub(cumulativeQuantity)
		}

		position.Data.Mark = executionData.LastPrice
		position.status = orderStatuses[executionData.OrderStatus]
		matched = true
	}

	if matched {
		if err := position.Execution(execution); err != nil {
			position.status = types.ERROR
			return
		}

		position.executions = append(position.executions, execution)
	}

	position.Publish()
}

func (position *Position) Execution(execution *kraken.Execution) error {
	if errnie.Error(kraken.Validate(execution)) != nil {
		position.status = types.ERROR
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"invalid execution",
			nil,
		))
	}

	feeRate, err := position.price.FeeRate(position.Data.Symbol)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get fee rate",
			err,
		))
	}

	position.Data.FeeRate, err = decimal.NewFromString(feeRate.Fee)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to parse fee rate: "+err.Error(),
			err,
		))
	}

	return nil
}

func (position *Position) Enter() error {
	position.requestedQty = position.Data.Qty
	amount, err := position.price.Taker(
		position.Data.Symbol, position.requestedQty,
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
TickerAck applies executable public quotes directly to this position.
*/
func (position *Position) TickerAck(buf []byte) {
	ticker := kraken.NewTicker(buf)

	if errnie.Error(kraken.Validate(ticker)) != nil {
		return
	}

	for _, tickerData := range ticker.Data {
		if tickerData.Symbol != position.Data.Symbol {
			continue
		}

		stop, triggered, err := position.valuation.Update(
			position.Data, position.Stop, tickerData,
		)

		if errnie.Error(err) != nil {
			return
		}

		position.Stop = stop
		position.Publish()

		if triggered && position.Status() != types.PENDING {
			errnie.Error(position.Exit())
		}
	}
}
