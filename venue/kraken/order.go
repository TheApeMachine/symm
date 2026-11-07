package kraken

import (
	"encoding/json"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/nomagique/market"
)

/*
OrderResult identifies the venue order created by Kraken.
*/
type OrderResult struct {
	OrderID      string `json:"order_id"`
	ClOrdID      string `json:"cl_ord_id,omitempty"`
	OrderUserref int    `json:"order_userref"`
}

/*
Order is Kraken's acknowledgement of an order request.
*/
type Order struct {
	Method   string      `json:"method"`
	Result   OrderResult `json:"result"`
	Error    string      `json:"error"`
	Success  bool        `json:"success"`
	ReqID    int64       `json:"req_id"`
	TimeIn   time.Time   `json:"time_in"`
	TimeOut  time.Time   `json:"time_out"`
	onChange func(market.MarketType)
}

func NewOrder() *Order {
	return &Order{
		Method: "add_order",
	}
}

/*
NewOrderFromMap builds a wire Order from a paper CLI acknowledgement.
*/
func NewOrderFromMap(model map[string]any) json.Marshaler {
	out := NewOrder()
	orderID, _ := model["order_id"].(string)
	success := false

	if value, ok := model["success"].(bool); ok {
		success = value
	}

	if !success && orderID != "" {
		success = true
	}

	out.Result.OrderID = orderID
	out.Success = success

	return out
}

func (order *Order) Request() map[string]any {
	return map[string]any{
		"method": "add_order",
	}
}

func (order *Order) OnChange(fn func(market.MarketType)) {
	order.onChange = fn
}

func (order *Order) MarshalJSON() ([]byte, error) {
	type alias Order
	return sonic.Marshal((*alias)(order))
}

func (order *Order) UnmarshalJSON(data []byte) error {
	type alias Order
	var decoded alias

	if err := sonic.Unmarshal(data, &decoded); err != nil {
		return err
	}

	onChange := order.onChange
	*order = Order(decoded)
	order.onChange = onChange

	if order.onChange != nil {
		order.onChange(order)
	}

	return nil
}

func (order *Order) ToMarket(other market.MarketType) market.MarketType {
	ack, ok := other.(*market.OrderAck)

	if !ok || ack == nil || order == nil {
		return other
	}

	out := market.NewOrderAck(map[string]any{
		"method":   order.Method,
		"order_id": order.Result.OrderID,
		"success":  order.Success,
		"error":    order.Error,
		"req_id":   order.ReqID,
	})
	out.TimeIn = order.TimeIn
	out.TimeOut = order.TimeOut

	return out.ToMarket(ack)
}

func (order *Order) FromMarket(other market.MarketType) market.MarketType {
	ack, ok := other.(*market.OrderAck)

	if !ok || ack == nil || order == nil {
		return order
	}

	order.Method = ack.Method
	order.Result.OrderID = ack.OrderID
	order.Success = ack.Success
	order.Error = ack.Error
	order.ReqID = ack.ReqID
	order.TimeIn = ack.TimeIn
	order.TimeOut = ack.TimeOut

	return order
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

/*
Command is the paper CLI argv for this add_order.
*/
func (req *AddOrderRequest) Command() []string {
	command := []string{req.Type, req.Pair, req.Volume}

	if req.OrdType == "limit" && req.Price != "" {
		command = append(command, "--type", "limit", "--price", req.Price)
	}

	if req.ClOrdId != "" {
		command = append(command, "--cl-ord-id", req.ClOrdId)
	}

	return command
}
