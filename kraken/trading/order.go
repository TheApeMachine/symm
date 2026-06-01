package trading

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
)

const (
	MethodAddOrder     = "add_order"
	MethodAmendOrder   = "amend_order"
	MethodCancelOrder  = "cancel_order"
	MethodCancelOrders = "cancel_orders"
	MethodBatchAdd     = "batch_add_orders"
	MethodBatchCancel  = "batch_cancel_orders"
	MethodEditOrder    = "edit_order"
)

/*
Side and OrderType mirror Kraken WebSocket v2 add_order params.
See https://docs.kraken.com/api/docs/websocket-v2/add_order/
*/
type OrderType string

const (
	Limit           OrderType = "limit"
	Market          OrderType = "market"
	StopLoss        OrderType = "stop-loss"
	StopLossLimit   OrderType = "stop-loss-limit"
	TakeProfit      OrderType = "take-profit"
	TakeProfitLimit OrderType = "take-profit-limit"
)

type Side string

const (
	Buy  Side = "buy"
	Sell Side = "sell"
)

/*
Triggers holds stop/take-profit trigger params for triggered order types.
*/
type Triggers struct {
	Reference string  `json:"reference,omitempty"`
	Price     float64 `json:"price"`
	PriceType string  `json:"price_type,omitempty"`
}

/*
AddParams is the params object on an add_order frame.
The authenticated socket injects token before send.
*/
type AddParams struct {
	OrderType  OrderType `json:"order_type"`
	Side       Side      `json:"side"`
	Symbol     string    `json:"symbol"`
	OrderQty   float64   `json:"order_qty,omitempty"`
	LimitPrice float64   `json:"limit_price,omitempty"`
	ClOrdID    string    `json:"cl_ord_id,omitempty"`
	Triggers   *Triggers `json:"triggers,omitempty"`
	Token      string    `json:"token,omitempty"`
}

/*
Ack is the add_order method response envelope.
*/
type Ack struct {
	Method  string `json:"method"`
	Success bool   `json:"success"`
	Error   string `json:"error"`
	ReqID   int    `json:"req_id"`
	Result  struct {
		OrderID string `json:"order_id"`
		ClOrdID string `json:"cl_ord_id"`
	} `json:"result"`
}

/*
Client sends order frames to the orders pool channel.
private.WebSocket picks them up and forwards to Kraken.
*/
type Client struct {
	ctx    context.Context
	cancel context.CancelFunc
	orders *qpool.BroadcastGroup
}

func NewOrder(ctx context.Context, pool *qpool.Q) (*Client, error) {
	ctx, cancel := context.WithCancel(ctx)

	orderClient := &Client{
		ctx:    ctx,
		cancel: cancel,
		orders: pool.CreateBroadcastGroup(public.OrdersChannel, 10*time.Millisecond),
	}

	return orderClient, errnie.Error(errnie.Require(map[string]any{
		"ctx":    orderClient.ctx,
		"cancel": orderClient.cancel,
		"orders": orderClient.orders,
	}))
}

func (client *Client) send(payload fiber.Map) error {
	client.orders.Send(&qpool.QValue[any]{Value: payload})
	return nil
}

func (client *Client) AddOrder(params AddParams) error {
	return errnie.Error(client.send(fiber.Map{
		"method": MethodAddOrder,
		"params": params,
	}))
}

func (client *Client) AmendOrder(orderID string) error {
	return errnie.Error(client.send(fiber.Map{
		"method": MethodAmendOrder,
		"params": fiber.Map{"order_id": orderID},
	}))
}

func (client *Client) CancelOrder(orderID string) error {
	return errnie.Error(client.send(fiber.Map{
		"method": MethodCancelOrder,
		"params": fiber.Map{"order_id": orderID},
	}))
}

func (client *Client) CancelAll() error {
	return errnie.Error(client.send(fiber.Map{
		"method": MethodCancelOrders,
		"params": fiber.Map{"cancel_all": true},
	}))
}

func (client *Client) CancelOnDisconnect() error {
	return errnie.Error(client.send(fiber.Map{
		"method": MethodCancelOrders,
		"params": fiber.Map{"cancel_on_disconnect": true},
	}))
}

func (client *Client) BatchAdd(params []AddParams) error {
	return errnie.Error(client.send(fiber.Map{
		"method": MethodBatchAdd,
		"params": params,
	}))
}

func (client *Client) BatchCancel(orderIDs []string) error {
	return errnie.Error(client.send(fiber.Map{
		"method": MethodBatchCancel,
		"params": orderIDs,
	}))
}

func (client *Client) EditOrder(orderID string, params AddParams) error {
	editParams := fiber.Map{"order_id": orderID}

	if params.OrderQty > 0 {
		editParams["order_qty"] = params.OrderQty
	}

	if params.LimitPrice > 0 {
		editParams["limit_price"] = params.LimitPrice
	}

	if params.Triggers != nil {
		editParams["triggers"] = params.Triggers
	}

	return errnie.Error(client.send(fiber.Map{
		"method": MethodEditOrder,
		"params": editParams,
	}))
}

func (client *Client) Close() error {
	client.cancel()
	return nil
}
