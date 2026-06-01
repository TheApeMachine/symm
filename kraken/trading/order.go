package trading

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
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
Client sends add_order frames through kraken.Client.
Paper and live selection lives in kraken/client.go.
*/
type Client struct {
	ctx    context.Context
	cancel context.CancelFunc
	conn   *kraken.Client
}

func NewOrder(ctx context.Context) (*Client, error) {
	ctx, cancel := context.WithCancel(ctx)

	conn := errnie.Does(func() (*kraken.Client, error) {
		return kraken.NewClient(ctx)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	orderClient := &Client{
		ctx:    ctx,
		cancel: cancel,
		conn:   conn,
	}

	return orderClient, errnie.Error(errnie.Require(map[string]any{
		"ctx":    orderClient.ctx,
		"cancel": orderClient.cancel,
		"conn":   orderClient.conn,
	}))
}

func (client *Client) AddOrder(params AddParams) error {
	return errnie.Error(client.conn.Send(public.OrdersChannel, fiber.Map{
		"method": MethodAddOrder,
		"params": params,
	}))
}

func (client *Client) AmendOrder(orderID string) error {
	return errnie.Error(client.conn.Send(public.OrdersChannel, fiber.Map{
		"method": MethodAmendOrder,
		"params": fiber.Map{
			"order_id": orderID,
		},
	}))
}

func (client *Client) CancelOrder(orderID string) error {
	return errnie.Error(client.conn.Send(public.OrdersChannel, fiber.Map{
		"method": MethodCancelOrder,
		"params": fiber.Map{
			"order_id": orderID,
		},
	}))
}

func (client *Client) CancelAll() error {
	return errnie.Error(client.conn.Send(public.OrdersChannel, fiber.Map{
		"method": MethodCancelOrders,
		"params": fiber.Map{
			"cancel_all": true,
		},
	}))
}

func (client *Client) CancelOnDisconnect() error {
	return errnie.Error(client.conn.Send(public.OrdersChannel, fiber.Map{
		"method": MethodCancelOrders,
		"params": fiber.Map{
			"cancel_on_disconnect": true,
		},
	}))
}

func (client *Client) BatchAdd(params []AddParams) error {
	return errnie.Error(client.conn.Send(public.OrdersChannel, fiber.Map{
		"method": MethodBatchAdd,
		"params": params,
	}))
}

func (client *Client) BatchCancel(orderIDs []string) error {
	return errnie.Error(client.conn.Send(public.OrdersChannel, fiber.Map{
		"method": MethodBatchCancel,
		"params": orderIDs,
	}))
}

func (client *Client) EditOrder(orderID string, params AddParams) error {
	return errnie.Error(client.conn.Send(public.OrdersChannel, fiber.Map{
		"method": MethodEditOrder,
		"params": fiber.Map{
			"order_id": orderID,
		},
	}))
}

func (client *Client) Close() error {
	client.cancel()

	return errnie.Error(client.conn.Close())
}
