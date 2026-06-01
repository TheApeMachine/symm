package order

import (
	"context"
	"encoding/json"

	"github.com/theapemachine/symm/kraken/private"
	"github.com/theapemachine/symm/kraken/trading"
)

type (
	Fill            = trading.Fill
	Ack             = trading.Ack
	ExecutionEvent  = trading.ExecutionEvent
	OrderType       = trading.OrderType
	Side            = trading.Side
	AddParams       = trading.AddParams
	ConditionalStop = trading.ConditionalStop
	CancelParams    = trading.CancelParams
	AmendParams     = trading.AmendParams
	Request         = trading.AddRequest
	CancelRequest   = trading.CancelRequest
)

const (
	MethodAddOrder    = trading.MethodAddOrder
	MethodAmendOrder  = trading.MethodAmendOrder
	MethodCancelOrder = trading.MethodCancelOrder
	Limit             = trading.Limit
	Market            = trading.Market
	Buy               = trading.Buy
	Sell              = trading.Sell
)

var NextClOrdID = trading.NextClOrdID

var (
	ParseExecutionFills  = trading.ParseExecutionFills
	ParseExecutionEvents = trading.ParseExecutionEvents
	ParseAck             = trading.ParseAck
	OrderFillTerminal    = trading.OrderFillTerminal
	FindOTOStopOrderID   = trading.FindOTOStopOrderID
)

var (
	MarketBuyCash  = trading.MarketBuyCash
	MarketSellBase = trading.MarketSellBase
	LimitBuyBid    = trading.LimitBuyBid
	CancelOrder    = trading.CancelOrder
)

/*
Client is the authenticated Kraken WebSocket v2 trading connection.
Deprecated: use private.WebSocket with trading.PublishAdd and trading execution parsers.
*/
type Client struct {
	socket *private.WebSocket
	fills  chan Fill
	acks   chan Ack
}

/*
NewClient wires private REST credentials for token refresh and order routing.
*/
func NewClient(ctx context.Context, apiKey, apiSecret string) (*Client, error) {
	socket, err := private.NewWebSocket(ctx, apiKey, apiSecret)

	if err != nil {
		return nil, err
	}

	client := &Client{
		socket: socket,
		fills:  make(chan Fill, 128),
		acks:   make(chan Ack, 64),
	}

	socket.OnMessage(client.dispatch)

	return client, nil
}

/*
Start dials the auth socket, subscribes to executions, and begins reading frames.
*/
func (client *Client) Start() error {
	return client.socket.Start()
}

/*
Publish sends one trading request frame with a fresh token when needed.
*/
func (client *Client) Publish(request Request) error {
	return trading.PublishAdd(client.socket, request)
}

/*
PublishCancel sends one cancel_order frame with a fresh token when needed.
*/
func (client *Client) PublishCancel(request CancelRequest) error {
	return trading.PublishCancel(client.socket, request)
}

func (client *Client) Fills() <-chan Fill {
	return client.fills
}

func (client *Client) Acks() <-chan Ack {
	return client.acks
}

/*
Close shuts down the trading client and returns any connection close error.
*/
func (client *Client) Close() error {
	return client.socket.Close()
}

func (client *Client) dispatch(payload []byte) {
	var envelope struct {
		Channel string `json:"channel"`
		Method  string `json:"method"`
	}

	if err := unmarshalEnvelope(payload, &envelope); err != nil {
		return
	}

	if envelope.Channel == "executions" {
		fills, err := ParseExecutionFills(payload)

		if err != nil {
			return
		}

		client.enqueueFills(fills)

		return
	}

	if envelope.Method == "" {
		return
	}

	ack, err := ParseAck(payload)

	if err != nil {
		return
	}

	client.enqueueAck(*ack)
}

func (client *Client) enqueueFills(fills []Fill) {
	for _, fill := range fills {
		if !client.enqueueFill(fill) {
			return
		}
	}
}

func (client *Client) enqueueFill(fill Fill) bool {
	if client.socket == nil {
		client.fills <- fill

		return true
	}

	select {
	case client.fills <- fill:
		return true
	case <-client.socket.Context().Done():
		return false
	}
}

func (client *Client) enqueueAck(ack Ack) {
	if client.socket == nil {
		client.acks <- ack

		return
	}

	select {
	case client.acks <- ack:
	case <-client.socket.Context().Done():
	}
}

func unmarshalEnvelope(payload []byte, envelope *struct {
	Channel string `json:"channel"`
	Method  string `json:"method"`
}) error {
	return json.Unmarshal(payload, envelope)
}
