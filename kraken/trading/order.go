package trading

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/public"
)

type OrderType string

const (
	Limit             OrderType = "limit"
	Market            OrderType = "market"
	Iceberg           OrderType = "iceberg"
	StopLoss          OrderType = "stop-loss"
	StopLossLimit     OrderType = "stop-loss-limit"
	TakeProfit        OrderType = "take-profit"
	TakeProfitLimit   OrderType = "take-profit-limit"
	TrailingStop      OrderType = "trailing-stop"
	TrailingStopLimit OrderType = "trailing-stop-limit"
	SettlePosition    OrderType = "settle-position"
)

type Side string

const (
	Buy  Side = "buy"
	Sell Side = "sell"
)

type Order struct {
	ID        string    `json:"id"`
	Symbol    string    `json:"symbol"`
	Side      Side      `json:"side"`
	Price     float64   `json:"price"`
	Quantity  float64   `json:"quantity"`
	Timestamp time.Time `json:"timestamp"`
}

type OrderRequest struct {
	Nonce     int    `json:"nonce"`
	Ordertype string `json:"ordertype"`
	Type      string `json:"type"`
	Volume    string `json:"volume"`
	Pair      string `json:"pair"`
	Price     string `json:"price"`
	ClOrdID   string `json:"cl_ord_id"`
}

type OrderMessage struct {
	Method    string `json:"method"`
	OrderType string `json:"ordertype"`
}

type Client struct {
	ctx           context.Context
	cancel        context.CancelFunc
	err           error
	conn          *kraken.Client
	orderRequests map[string]*OrderRequest
	orderMessages map[string]*OrderMessage
}

func NewOrder(
	ctx context.Context,
) (*Client, error) {
	ctx, cancel := context.WithCancel(ctx)

	client := &Client{
		ctx:    ctx,
		cancel: cancel,
		conn: errnie.Does(func() (*kraken.Client, error) {
			return kraken.NewClient(ctx)
		}).Or(func(err error) {
			errnie.Error(err)
		}).Value(),
		orderRequests: make(map[string]*OrderRequest),
		orderMessages: make(map[string]*OrderMessage),
	}

	return client, errnie.Error(errnie.Require(map[string]any{
		"ctx":           client.ctx,
		"cancel":        client.cancel,
		"conn":          client.conn,
		"orderMessages": client.orderMessages,
	}))
}

func (client *Client) Add(order *OrderRequest) error {
	return errnie.Error(client.conn.Send(public.OrdersChannel, order))
}

func (client *Client) Amend(order *OrderRequest) error {
	return errnie.Error(client.conn.Send(public.OrdersChannel, order))
}

func (client *Client) Cancel(order *OrderRequest) error {
	return errnie.Error(client.conn.Send(public.OrdersChannel, order))
}

func (client *Client) CancelAll() error {
	return errnie.Error(client.conn.Send(public.OrdersChannel, "cancel_all"))
}

func (client *Client) CancelOnDisconnect() error {
	return errnie.Error(client.conn.Send(public.OrdersChannel, "cancel_on_disconnect"))
}

func (client *Client) BatchAdd(orders []*OrderRequest) error {
	return errnie.Error(client.conn.Send(public.OrdersChannel, orders))
}

func (client *Client) BatchCancel(orders []*OrderRequest) error {
	return errnie.Error(client.conn.Send(public.OrdersChannel, orders))
}

func (client *Client) Edit(order *OrderRequest) error {
	return errnie.Error(client.conn.Send(public.OrdersChannel, order))
}
