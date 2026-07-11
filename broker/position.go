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
	Symbol         string           `json:"symbol"`
	Qty            float64          `json:"qty"`
	EntryPrice     decimal.Decimal  `json:"entry_price"`
	Mark           decimal.Decimal  `json:"mark"`
	PnL            decimal.Decimal  `json:"pnl"`
	ReturnPct      float64          `json:"return_pct"`
	FeeRate        *decimal.Decimal `json:"fee_rate,omitempty"`
	Spread         decimal.Decimal  `json:"spread"`
	PriceIncrement decimal.Decimal  `json:"price_increment"`
}

type Position struct {
	status     types.Status
	api        *websocket.API
	ui         chan []byte
	price      *Price
	balance    *Balance
	orderID    string
	clientID   int
	reqID      int
	order      *kraken.LimitOrder
	executions []*kraken.Execution
	Data       *PositionData
	tickers    []*kraken.TickerData
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

	return position
}

func (position *Position) Status() types.Status {
	return position.status
}

func (position *Position) Publish() {
	select {
	case position.ui <- datura.Map[any]{
		"positions": []PositionData{*position.Data},
	}.Marshal():
	default:
	}
}

func (position *Position) OrderAck(buf []byte) {
	orderAck := kraken.NewOrderResponse(buf)

	if errnie.Error(kraken.Validate(orderAck)) != nil {
		position.status = types.ERROR
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

	for _, executionData := range execution.Data {
		if executionData.OrderID == position.orderID {
			position.Data.Qty += executionData.LastQty
			position.Data.EntryPrice = executionData.LastPrice
			position.Data.Mark = executionData.LastPrice
			position.executions = append(position.executions, execution)
			position.status = orderStatuses[executionData.OrderStatus]
			return
		}
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
	amount, err := position.price.Taker(
		position.Data.Symbol, position.Data.Qty,
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

	if err := position.api.AddOrder(kraken.NewMarketOrder(
		"buy",
		position.Data.Qty,
		position.Data.Symbol,
	)); err != nil {
		position.status = types.ERROR

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	position.status = types.PENDING
	position.Publish()
	return nil
}

func (position *Position) Exit() error {
	if err := position.api.AddOrder(kraken.NewMarketOrder(
		"sell",
		position.Data.Qty,
		position.Data.Symbol,
	)); err != nil {
		position.status = types.ERROR
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	position.status = types.PENDING
	position.Publish()
	return nil
}

func (position *Position) Executions() []*kraken.Execution {
	return position.executions
}
