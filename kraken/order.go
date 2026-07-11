package kraken

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

type Order struct {
	Method string `json:"method"`
	Params any    `json:"params"`
	ReqID  int    `json:"req_id"`
}

func (order *Order) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(struct {
		Method string `json:"method"`
		Params any    `json:"params"`
		ReqID  int    `json:"req_id"`
	}{
		Method: order.Method,
		Params: order.Params,
		ReqID:  order.ReqID,
	})
}

type OrderResponseResult struct {
	OrderID      string `json:"order_id"`
	OrderUserref int    `json:"order_userref"`
}

type OrderResponse struct {
	Method  string              `json:"method"`
	Result  OrderResponseResult `json:"result"`
	Error   string              `json:"error"`
	Success bool                `json:"success"`
	ReqID   int                 `json:"req_id"`
	TimeIn  time.Time           `json:"time_in"`
	TimeOut time.Time           `json:"time_out"`
}

func NewOrderResponse(buf []byte) *OrderResponse {
	data := &OrderResponse{}
	errnie.Error(sonic.Unmarshal(buf, data))
	return data
}

func (order *OrderResponse) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(order)
}

func (order *OrderResponse) Action() string {
	return order.Method
}

func (order *OrderResponse) IsSuccess() bool {
	return order.Success
}

type LimitOrder struct {
	Method string           `json:"method"`
	Params LimitOrderParams `json:"params"`
	ReqID  int              `json:"req_id"`
}

type LimitOrderParams struct {
	OrderType    string  `json:"order_type"`
	Side         string  `json:"side"`
	LimitPrice   float64 `json:"limit_price,omitempty"`
	OrderUserref int     `json:"order_userref"`
	OrderQty     float64 `json:"order_qty"`
	Symbol       string  `json:"symbol"`
	Token        string  `json:"token"`
}

func NewLimitOrder(
	side string,
	limitPrice float64,
	orderQty float64,
	symbol string,
) *LimitOrder {
	return &LimitOrder{
		Method: "add_order",
		Params: LimitOrderParams{
			OrderType:  "limit",
			Side:       side,
			LimitPrice: limitPrice,
			OrderQty:   orderQty,
			Symbol:     symbol,
		},
		ReqID: int(time.Now().UnixNano()),
	}
}

type MarketOrder struct {
	Method string            `json:"method"`
	Params MarketOrderParams `json:"params"`
	ReqID  int               `json:"req_id"`
}

type MarketOrderParams struct {
	OrderType  string  `json:"order_type"`
	Side       string  `json:"side"`
	OrderQty   float64 `json:"order_qty"`
	Symbol     string  `json:"symbol"`
	LimitPrice float64 `json:"limit_price,omitempty"`
}

func NewMarketOrder(
	side string,
	orderQty float64,
	symbol string,
) *MarketOrder {
	return &MarketOrder{
		Method: "add_order",
		Params: MarketOrderParams{
			OrderType: "market",
			Side:      side,
			OrderQty:  orderQty,
			Symbol:    symbol,
		},
		ReqID: int(time.Now().UnixNano()),
	}
}

func (order *MarketOrder) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(order)
}

func (order *LimitOrder) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(order)
}

func NewOrderResponseFromMap(model datura.Map[any], reqID int) *OrderResponse {
	orderID, _ := model["order_id"].(string)

	return &OrderResponse{
		Method: "add_order",
		Result: OrderResponseResult{
			OrderID: orderID,
		},
		Success: true,
		ReqID:   reqID,
	}
}
