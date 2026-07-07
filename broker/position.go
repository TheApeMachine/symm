package broker

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

var statusMap = map[string]types.Status{
	"new":              types.NEW,
	"partially_filled": types.PARTIAL,
	"filled":           types.FILLED,
	"canceled":         types.CANCELED,
	"expired":          types.ERROR,
}

type PositionData struct {
	Symbol     string          `json:"symbol"`
	Qty        float64         `json:"qty"`
	EntryPrice decimal.Decimal `json:"entry_price"`
	Mark       decimal.Decimal `json:"mark"`
	PnL        decimal.Decimal `json:"pnl"`
	ReturnPct  float64         `json:"return_pct"`
}

type Position struct {
	status     types.Status
	private    websocket.Private
	orderID    string
	order      *kraken.OrderData
	executions []*kraken.ExecutionData
	data       *PositionData
	tickers    []*kraken.TickerData
}

func NewPosition(
	private websocket.Private,
	data *PositionData,
) *Position {
	return &Position{
		status:     types.INITIALIZING,
		private:    private,
		data:       data,
		executions: make([]*kraken.ExecutionData, 0),
		tickers:    make([]*kraken.TickerData, 0),
	}
}

func (position *Position) OrderAck(orderAck *kraken.OrderResponse) error {
	position.status = types.OPEN
	return nil
}

func (position *Position) Order(order *kraken.OrderData) error {
	position.order = order
	return nil
}

func (position *Position) Execution(execution *kraken.ExecutionData) error {
	position.executions = append(position.executions, execution)
	position.status = statusMap[execution.OrderStatus]
	return nil
}

func (position *Position) AddTicker(ticker *kraken.TickerData) error {
	position.tickers = append(position.tickers, ticker)
	return nil
}

func (position *Position) Enter() error {
	err := errnie.Error(
		position.private.Submit(&kraken.Order{
			Method: "add_order",
			Params: kraken.LimitOrderParams{
				OrderType: "market",
				Side:      "buy",
				OrderQty:  position.data.Qty,
				Symbol:    position.data.Symbol,
			},
			ReqID: int(time.Now().UnixNano()),
		}),
	)

	if err != nil {
		position.status = types.ERROR
		return err
	}

	position.status = types.PENDING
	return nil
}

func (position *Position) Exit() error {
	err := position.private.Submit(&kraken.Order{
		Method: "add_order",
		Params: kraken.LimitOrderParams{
			OrderType: "market",
			Side:      "sell",
			OrderQty:  position.data.Qty,
			Symbol:    position.data.Symbol,
		},
		ReqID: int(time.Now().UnixNano()),
	})

	if err != nil {
		position.status = types.ERROR
		return err
	}

	position.status = types.PENDING
	return nil
}
