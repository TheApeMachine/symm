package websocket

import (
	"context"
	"os"
	"strings"

	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

type Account struct {
	ctx        context.Context
	cancel     context.CancelFunc
	client     *spot.WebSocket
	url        string
	publicKey  string
	privateKey string
	observers  []chan map[string]any
	buffer     int
}

func NewAccount(ctx context.Context) *Account {
	ctx, cancel := context.WithCancel(ctx)

	account := &Account{
		ctx:        ctx,
		cancel:     cancel,
		client:     spot.NewWebSocket(),
		url:        os.Getenv("KRAKEN_API_SPOT_WS_AUTH_URL"),
		publicKey:  os.Getenv("KRAKEN_API_KEY"),
		privateKey: os.Getenv("KRAKEN_API_SECRET"),
		buffer:     viper.GetViper().GetInt("system.websocket.channel.buffer"),
	}

	account.configure()

	account.client.OnSent.Recurring(func(e *callback.Event[*kraken.WebSocketMessage]) {
		account.checkContext()
		out, err := e.Data.Map()

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				err.Error(),
				err,
			))
		}

		for _, observer := range account.observers {
			observer <- out
		}
	})

	account.client.OnReceived.Recurring(func(e *callback.Event[*kraken.WebSocketMessage]) {
		account.checkContext()
		out, err := e.Data.Map()

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				err.Error(),
				err,
			))
		}

		for _, observer := range account.observers {
			observer <- out
		}
	})

	account.client.OnAuthenticated.Recurring(func(e *callback.Event[string]) {
		account.checkContext()
		errnie.Error(account.client.SubBalances())
		errnie.Error(account.client.SubExecutions())
	})

	account.client.OnConnected.Recurring(func(e *callback.Event[any]) {
		account.checkContext()
		errnie.Error(account.client.Authenticate())
	})

	errnie.Error(account.client.Connect())

	return account
}

func (account *Account) Observe() chan map[string]any {
	out := make(chan map[string]any, account.buffer)
	account.observers = append(account.observers, out)
	return out
}

func (account *Account) Submit(artifact *datura.Artifact) error {
	request, err := NewOrderRequest(artifact)
	if err != nil {
		return err
	}

	switch request.Method {
	case "add_order":
		return account.addOrder(request)
	case "cancel_order":
		return account.client.CancelOrder(map[string]any{
			"params": request.Params,
			"req_id": request.ReqID,
		})
	default:
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"account: unsupported private request method: "+request.Method,
			nil,
		))
	}
}

func (account *Account) Sync() error {
	return nil
}

func (account *Account) configure() {
	if strings.TrimSpace(account.url) != "" {
		account.client.URL = account.url
	}

	if account.publicKey == "" {
		account.publicKey = os.Getenv("SYMM_KRAKEN_API_KEY")
	}

	if account.privateKey == "" {
		account.privateKey = os.Getenv("SYMM_KRAKEN_API_SECRET")
	}

	account.client.REST.PublicKey = account.publicKey
	account.client.REST.PrivateKey = account.privateKey
}

func (account *Account) addOrder(request *OrderRequest) error {
	quantity, err := request.Float("order_qty")
	if err != nil {
		return err
	}

	return account.client.AddOrder(
		request.String("order_type"),
		request.String("side"),
		quantity,
		request.String("symbol"),
		map[string]any{
			"params": request.Params,
			"req_id": request.ReqID,
		},
	)
}

func (account *Account) checkContext() {
	select {
	case <-account.ctx.Done():
		account.Close()
	default:
	}
}

func (account *Account) Close() {
	account.cancel()
	account.client.Disconnect()
}
