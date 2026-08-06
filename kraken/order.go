package kraken

import (
	"encoding/json"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

/*
Order is the generic authenticated Kraken request envelope.
*/
type Order struct {
	Method string `json:"method"`
	Params any    `json:"params"`
	ReqID  int64  `json:"req_id"`
}

/*
MarshalJSON encodes the generic order envelope for the websocket transport.
*/
func (order *Order) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(struct {
		Method string `json:"method"`
		Params any    `json:"params"`
		ReqID  int64  `json:"req_id"`
	}{
		Method: order.Method,
		Params: order.Params,
		ReqID:  order.ReqID,
	})
}

/*
OrderResponseResult identifies the venue order created by Kraken.
*/
type OrderResponseResult struct {
	OrderID      string `json:"order_id"`
	OrderUserref int    `json:"order_userref"`
}

/*
OrderResponse is Kraken's acknowledgement of an order request.
*/
type OrderResponse struct {
	Method  string              `json:"method"`
	Result  OrderResponseResult `json:"result"`
	Error   string              `json:"error"`
	Success bool                `json:"success"`
	ReqID   int64               `json:"req_id"`
	TimeIn  time.Time           `json:"time_in"`
	TimeOut time.Time           `json:"time_out"`
}

/*
NewOrderResponse decodes one order acknowledgement from Kraken.
*/
func NewOrderResponse(buf []byte) *OrderResponse {
	data := &OrderResponse{}
	errnie.Error(sonic.Unmarshal(buf, data))
	return data
}

/*
MarshalJSON encodes an order acknowledgement for paper and test transports.
*/
func (order *OrderResponse) MarshalJSON() ([]byte, error) {
	type alias OrderResponse
	return sonic.Marshal((*alias)(order))
}

/*
Action returns the transport method used to route this acknowledgement.
*/
func (order *OrderResponse) Action() string {
	return order.Method
}

/*
IsSuccess reports whether Kraken accepted the order request.
*/
func (order *OrderResponse) IsSuccess() bool {
	return order.Success
}

/*
LimitOrder is an exact fixed-point limit order request envelope.
*/
type LimitOrder struct {
	Method string           `json:"method"`
	Params LimitOrderParams `json:"params"`
	ReqID  int64            `json:"req_id"`
}

/*
LimitOrderParams carries exact decimal text as JSON numbers so the venue sees
numeric fields without an intervening float64 conversion.
*/
type LimitOrderParams struct {
	OrderType    string      `json:"order_type"`
	Side         string      `json:"side"`
	LimitPrice   json.Number `json:"limit_price,omitempty"`
	OrderUserref int         `json:"order_userref"`
	OrderQty     json.Number `json:"order_qty"`
	Symbol       string      `json:"symbol"`
	Token        string      `json:"token"`
}

/*
NewLimitOrder creates a limit request from exact decimal price and quantity.
*/
func NewLimitOrder(
	side string,
	limitPrice *decimal.Decimal,
	orderQty *decimal.Decimal,
	symbol string,
) *LimitOrder {
	return &LimitOrder{
		Method: "add_order",
		Params: LimitOrderParams{
			OrderType:  "limit",
			Side:       side,
			LimitPrice: json.Number(limitPrice.String()),
			OrderQty:   json.Number(orderQty.String()),
			Symbol:     symbol,
		},
		ReqID: orderRequestID.Next(),
	}
}

/*
MarketOrder is an exact fixed-point market order request envelope.
*/
type MarketOrder struct {
	Method string            `json:"method"`
	Params MarketOrderParams `json:"params"`
	ReqID  int64             `json:"req_id"`
}

/*
MarketOrderParams retains executable quantity text as a JSON number.
*/
type MarketOrderParams struct {
	OrderType  string           `json:"order_type"`
	Side       string           `json:"side"`
	Symbol     string           `json:"symbol"`
	OrderQty   *decimal.Decimal `json:"order_qty"`
	LimitPrice *decimal.Decimal `json:"limit_price,omitempty"`
}

/*
NewMarketOrder creates a market request without converting quantity to float64.
*/
func NewMarketOrder(
	side string,
	symbol string,
	orderQty *decimal.Decimal,
) *MarketOrder {
	return &MarketOrder{
		Method: "add_order",
		Params: MarketOrderParams{
			OrderType: "market",
			Side:      side,
			OrderQty:  orderQty,
			Symbol:    symbol,
		},
		ReqID: orderRequestID.Next(),
	}
}

/*
MarshalJSON encodes the exact market order for Kraken's numeric wire schema.
*/
func (order *MarketOrder) MarshalJSON() ([]byte, error) {
	type alias MarketOrder
	return sonic.Marshal((*alias)(order))
}

/*
MarshalJSON encodes the exact limit order for Kraken's numeric wire schema.
*/
func (order *LimitOrder) MarshalJSON() ([]byte, error) {
	type alias LimitOrder
	return sonic.Marshal((*alias)(order))
}

/*
NewOrderResponseFromMap adapts a paper fill acknowledgement to Kraken's model.
*/
func NewOrderResponseFromMap(model datura.Map[any], reqID int64) *OrderResponse {
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
