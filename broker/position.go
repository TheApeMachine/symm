package broker

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
)

type PositionData struct {
	Symbol     string          `json:"symbol"`
	Qty        float64         `json:"qty"`
	EntryPrice decimal.Decimal `json:"entry_price"`
	Mark       decimal.Decimal `json:"mark"`
	PnL        decimal.Decimal `json:"pnl"`
	ReturnPct  float64         `json:"return_pct"`
}

type Position struct {
	private   websocket.Private
	order     *kraken.Order
	orderAck  *kraken.OrderResponse
	openOrder *kraken.OrderData
	execution *kraken.ExecutionData
	data      *PositionData
	tickers   []*kraken.TickerData
}

func NewPosition(
	private websocket.Private,
	data *PositionData,
) *Position {
	return &Position{
		private: private,
		data:    data,
		tickers: make([]*kraken.TickerData, 0),
	}
}

func (position *Position) OrderAck(orderAck *kraken.OrderResponse) error {
	position.orderAck = orderAck
	return nil
}

func (position *Position) OpenOrder(order *kraken.OrderData) error {
	position.openOrder = order
	return nil
}

func (position *Position) Execution(execution *kraken.ExecutionData) error {
	position.execution = execution
	return nil
}

func (position *Position) AddTicker(ticker *kraken.TickerData) error {
	position.tickers = append(position.tickers, ticker)
	return nil
}

func (position *Position) Enter() error {
	position.order = &kraken.Order{
		Method: "add_order",
		Params: kraken.LimitOrderParams{
			OrderType: "market",
			Side:      "buy",
			OrderQty:  position.data.Qty,
			Symbol:    position.data.Symbol,
		},
		ReqID: int(time.Now().UnixNano()),
	}

	return position.private.Submit(position.order)
}

func (position *Position) Exit() error {
	position.order = &kraken.Order{
		Method: "add_order",
		Params: kraken.LimitOrderParams{
			OrderType: "market",
			Side:      "sell",
			OrderQty:  position.data.Qty,
			Symbol:    position.data.Symbol,
		},
		ReqID: int(time.Now().UnixNano()),
	}

	return position.private.Submit(position.order)
}
