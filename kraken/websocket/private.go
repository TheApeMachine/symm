package websocket

import (
	"context"
	"os"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	kfx "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

type Private interface {
	Observe(string) chan []byte
	Submit(*kraken.Order) error
	Close()
}

func NewPrivate(ctx context.Context) Private {
	if viper.GetString("trading.model") == "live" {
		return NewLivePrivate(ctx)
	}

	return NewPaperPrivate(ctx)
}

type LivePrivate struct {
	ctx        context.Context
	cancel     context.CancelFunc
	client     *spot.WebSocket
	url        string
	publicKey  string
	privateKey string
	stream     *Stream
}

func NewLivePrivate(ctx context.Context) *LivePrivate {
	ctx, cancel := context.WithCancel(ctx)
	buffer := viper.GetViper().GetInt("system.websocket.channel.buffer")

	private := &LivePrivate{
		ctx:        ctx,
		cancel:     cancel,
		client:     spot.NewWebSocket(),
		url:        "ws-auth.kraken.com/v2",
		publicKey:  os.Getenv("KRAKEN_API_KEY"),
		privateKey: os.Getenv("KRAKEN_API_SECRET"),
		stream:     NewStream(buffer),
	}

	private.configure()

	private.client.OnSent.Recurring(func(e *callback.Event[*kfx.WebSocketMessage]) {
		private.checkContext()
	})

	private.client.OnReceived.Recurring(func(e *callback.Event[*kfx.WebSocketMessage]) {
		private.checkContext()
		private.stream.Receive(e.Data.Bytes())
	})

	private.client.OnAuthenticated.Recurring(func(e *callback.Event[string]) {
		private.checkContext()
		errnie.Error(private.client.SubBalances())
		errnie.Error(private.client.SubExecutions())
	})

	private.client.OnConnected.Recurring(func(e *callback.Event[any]) {
		private.checkContext()
		errnie.Error(private.client.Authenticate())
	})

	errnie.Error(private.client.Connect())

	return private
}

func (private *LivePrivate) Observe(channel string) chan []byte {
	observer := private.stream.Observe(channel)

	switch channel {
	case "balances":
		errnie.Error(private.publishBalances())
	case "executions":
		errnie.Error(private.publishExecutions())
	case "orders":
		errnie.Error(private.publishOrders())
	}

	return observer
}

func (private *LivePrivate) Submit(order *kraken.Order) error {
	if order == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken: private order required",
			nil,
		))
	}

	params := map[string]any{}
	raw, err := sonic.Marshal(order.Params)
	if err != nil {
		return err
	}

	if err := sonic.Unmarshal(raw, &params); err != nil {
		return err
	}

	switch strings.TrimSpace(order.Method) {
	case "add_order":
		orderType, _ := params["order_type"].(string)
		side, _ := params["side"].(string)
		quantity, _ := params["order_qty"].(float64)
		symbol, _ := params["symbol"].(string)

		if strings.TrimSpace(orderType) == "" ||
			strings.TrimSpace(side) == "" ||
			strings.TrimSpace(symbol) == "" ||
			quantity <= 0 {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken: live add_order requires order_type, side, symbol, and positive order_qty",
				nil,
			))
		}

		return private.client.AddOrder(
			strings.TrimSpace(orderType),
			strings.TrimSpace(side),
			quantity,
			strings.TrimSpace(symbol),
			map[string]any{"params": params, "req_id": order.ReqID},
		)
	case "cancel_order":
		return private.client.CancelOrder(map[string]any{
			"params": params,
			"req_id": order.ReqID,
		})
	}

	return errnie.Error(errnie.Err(
		errnie.Validation,
		"kraken: unsupported private method: "+order.Method,
		nil,
	))
}

func (private *LivePrivate) Close() {
	private.cancel()
	private.client.Disconnect()
}

func (private *LivePrivate) publishBalances() error {
	response, err := private.client.REST.Balances()

	if err != nil {
		return err
	}

	rows := kraken.NewBalanceDataSliceFromSpot(response.Result)
	return private.publish("balances", rows)
}

func (private *LivePrivate) publishExecutions() error {
	return private.publish("executions", kraken.ExecutionDataSlice{})
}

func (private *LivePrivate) publishOrders() error {
	response, err := private.client.REST.OpenOrders(&spot.OpenOrdersRequest{
		Trades: true,
	})

	if err != nil {
		return err
	}

	rows := kraken.NewOrderDataSliceFromSpot(response.Result.Open)
	return private.publish("orders", rows)
}

func (private *LivePrivate) publish(channel string, rows any) error {
	buf, err := sonic.Marshal(rows)

	if err != nil {
		return err
	}

	private.stream.Receive(append(
		[]byte(`{"channel":"`+channel+`","data":`),
		append(buf, '}')...,
	))

	return nil
}

func (private *LivePrivate) configure() {
	if strings.TrimSpace(private.url) != "" {
		private.client.URL = private.url
	}

	if private.publicKey == "" {
		private.publicKey = os.Getenv("SYMM_KRAKEN_API_KEY")
	}

	if private.privateKey == "" {
		private.privateKey = os.Getenv("SYMM_KRAKEN_API_SECRET")
	}

	private.client.REST.PublicKey = private.publicKey
	private.client.REST.PrivateKey = private.privateKey
}

func (private *LivePrivate) checkContext() {
	select {
	case <-private.ctx.Done():
		private.Close()
	default:
	}
}
