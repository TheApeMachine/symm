package kraken

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/data"
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
func NewOrderResponseFromMap(model datura.Map[any], reqID int64) *OrderResponse {
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
		ReqID:   reqID,
	}
}

type AddOrderRequest struct {
	Token   string `json:"token"`
	Pair    string `json:"symbol"`
	Type    string `json:"side"`
	OrdType string `json:"order_type"`
	// Volume is the venue size string. WS v2 names it order_qty (REST still
	// says volume); paper and live add_order both consume order_qty.
	Volume  string `json:"order_qty"`
	Price   string `json:"limit_price,omitempty"`
	ClOrdId string `json:"cl_ord_id,omitempty"`
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
ToValue serializes the AddOrderRequest into a *data.Value layer.
*/
func (req *AddOrderRequest) ToValue(epochs ...int64) *data.Value {
	if req == nil {
		return nil
	}

	val := data.NewValue("kraken", "order", req.Pair)
	if len(epochs) > 0 && epochs[0] > 0 {
		val.Epoch = uint64(epochs[0])
	}
	val.Append(req)
	return val
}
