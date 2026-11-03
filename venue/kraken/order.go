package kraken

import (
	"encoding/json"
	"time"

	"github.com/bytedance/sonic"
)

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
MarshalJSON encodes an order acknowledgement for paper and test transports.
*/
func (order *OrderResponse) MarshalJSON() ([]byte, error) {
	type alias OrderResponse
	return sonic.Marshal((*alias)(order))
}

func (order *OrderResponse) UnmarshalJSON(data []byte) error {
	type alias OrderResponse
	var decoded alias

	if err := sonic.Unmarshal(data, &decoded); err != nil {
		return err
	}

	*order = OrderResponse(decoded)
	return nil
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
NewOrderResponseFromMap adapts a paper fill acknowledgement to Kraken's model.
*/
func NewOrderResponseFromMap(model map[string]any) json.Marshaler {
	orderID, _ := model["order_id"].(string)
	success := false

	if s, ok := model["success"].(bool); ok {
		success = s
	}

	if !success && orderID != "" {
		success = true
	}

	return &OrderResponse{
		Method: "add_order",
		Result: OrderResponseResult{
			OrderID: orderID,
		},
		Success: success,
	}
}

type AddOrderRequest struct {
	Token   string `json:"token"`
	Pair    string `json:"symbol"`
	Type    string `json:"side"`
	OrdType string `json:"order_type"`
	// Volume is the venue size string. WS v2 names it order_qty (REST still
	// says volume); paper and live add_order both consume order_qty.
	Volume   string `json:"order_qty"`
	Price    string `json:"limit_price,omitempty"`
	ClOrdId  string `json:"cl_ord_id,omitempty"`
	PostOnly bool   `json:"post_only,omitempty"`
}

type AddOrderMessage struct {
	Method string          `json:"method"`
	Params AddOrderRequest `json:"params"`
}

func NewAddOrderMessage(token string, req *AddOrderRequest) AddOrderMessage {
	req.Token = token

	if req.OrdType == "" {
		req.OrdType = "market"
		if req.Price != "" {
			req.OrdType = "limit"
		}
	}

	return AddOrderMessage{
		Method: "add_order",
		Params: *req,
	}
}

func (msg AddOrderMessage) MarshalJSON() ([]byte, error) {
	type alias AddOrderMessage
	return sonic.Marshal((*alias)(&msg))
}

/*
CancelOrderRequest is the WS v2 cancel_order params body.
*/
type CancelOrderRequest struct {
	Token   string   `json:"token,omitempty"`
	OrderID []string `json:"order_id,omitempty"`
	ClOrdID []string `json:"cl_ord_id,omitempty"`
}

type CancelOrderMessage struct {
	Method string             `json:"method"`
	Params CancelOrderRequest `json:"params"`
}

func NewCancelOrderMessage(clOrdID string) CancelOrderMessage {
	return CancelOrderMessage{
		Method: "cancel_order",
		Params: CancelOrderRequest{ClOrdID: []string{clOrdID}},
	}
}

func (msg CancelOrderMessage) MarshalJSON() ([]byte, error) {
	type alias CancelOrderMessage
	return sonic.Marshal((*alias)(&msg))
}
