package trading

import (
	"context"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/paper"
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
	rest          public.RestClient
	ws            public.WebSocketClient
	orderRequests map[string]*OrderRequest
	orderMessages map[string]*OrderMessage
}

func NewOrder(
	ctx context.Context,
) (*Client, error) {
	ctx, cancel := context.WithCancel(ctx)

	rest := errnie.Does(func() (public.RestClient, error) {
		if viper.GetViper().Get("trading.model") == "paper" {
			return paper.NewRest(ctx)
		}

		return public.NewRest(ctx, public.EndpointAddOrder), nil
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	ws := errnie.Does(func() (public.WebSocketClient, error) {
		if viper.GetViper().Get("trading.model") == "paper" {
			return paper.NewWebSocket(ctx)
		}

		return public.NewWebSocket(ctx)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	client := &Client{
		ctx:           ctx,
		cancel:        cancel,
		rest:          rest,
		ws:            ws,
		orderRequests: make(map[string]*OrderRequest),
		orderMessages: make(map[string]*OrderMessage),
	}

	return client, errnie.Error(errnie.Require(map[string]any{
		"ctx":           client.ctx,
		"cancel":        client.cancel,
		"rest":          client.rest,
		"ws":            client.ws,
		"orderMessages": client.orderMessages,
	}))
}

func (order *OrderRequest) Add() error {
}

func (order *OrderRequest) Amend() error {
	return nil
}

func (order *OrderRequest) Cancel() error {
	return nil
}

func (order *OrderRequest) CancelAll() error {
	return nil
}

func (order *OrderRequest) CancelOnDisconnect() error {
	return nil
}

func (order *OrderRequest) BatchAdd() error {
	return nil
}

func (order *OrderRequest) BatchCancel() error {
	return nil
}

func (order *OrderRequest) Edit() error {
	return nil
}
