package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	gorillawebsocket "github.com/gorilla/websocket"
	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	sdkkraken "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

const (
	PublicWebSocketURL  = "wss://ws.kraken.com/v2"
	PrivateWebSocketURL = "wss://ws-auth.kraken.com/v2"
	Level3WebSocketURL  = "wss://ws-l3.kraken.com/v2"
)

var entityMap = map[string]func([]byte) any{
	"ticker":     func(buf []byte) any { return kraken.NewTicker(buf) },
	"book":       func(buf []byte) any { return kraken.NewBook(buf) },
	"trade":      func(buf []byte) any { return kraken.NewTrade(buf) },
	"level3":     func(buf []byte) any { return kraken.NewLevel3(buf) },
	"instrument": func(buf []byte) any { return kraken.NewInstrument(buf) },
	"status":     func(buf []byte) any { return func() bool { return true }() },
	"heartbeat":  func(buf []byte) any { return func() bool { return true }() },
}

/*
Live is one spot websocket session: SDK client, channel fan-out, auth/nonce,
and Sub* resubscribe after the SDK reconnects.
*/
type Live struct {
	status      types.Status
	ctx         context.Context
	cancel      context.CancelFunc
	client      *spot.WebSocket
	simulator   *Simulator
	book        *Book
	symbols     []string
	auth        bool
	nonce       *AuthNonce
	nonceErr    error
	subscribers *sync.Map
	callbacks   *sync.Map
	priceIncr   sync.Map
	resyncing   sync.Map
	paper       *Paper
	model       string
}

/*
New opens a spot websocket session and wires SDK callbacks in the constructor.
*/
func New(
	ctx context.Context,
	simulator *Simulator,
	auth bool,
	endpoint string,
) *Live {
	ctx, cancel := context.WithCancel(ctx)

	live := &Live{
		ctx:         ctx,
		cancel:      cancel,
		status:      types.INITIALIZING,
		simulator:   simulator,
		client:      spot.NewWebSocket(),
		auth:        auth,
		subscribers: &sync.Map{},
		callbacks:   &sync.Map{},
		paper:       NewPaper(ctx, NewSimulator()),
		model:       viper.GetViper().GetString("trading.model"),
	}

	live.client.URL = endpoint

	if auth {
		nonce, err := processAuthNonce()
		live.nonce = nonce
		live.nonceErr = err
		live.client.REST.PublicKey = os.Getenv("KRAKEN_API_KEY")
		live.client.REST.PrivateKey = os.Getenv("KRAKEN_API_SECRET")

		if live.nonceErr != nil || live.nonce == nil {
			return nil
		}

		// Private and every Level3 batch authenticate with the same key; they
		// must share one monotonic nonce sequence or concurrent token fetches
		// collide (EAPI:Invalid nonce).
		live.client.REST.Nonce = live.nonce.Next
	}

	if endpoint == Level3WebSocketURL {
		live.book = NewBook(ctx)
	}

	live.client.OnReceived.Recurring(func(event *callback.Event[*sdkkraken.WebSocketMessage]) {
		raw := event.Data.Bytes()

		channel := utils.GetString(raw, "channel")

		if channel == "" {
			if method := utils.GetString(raw, "method"); method == "add_order" {
				channel = method
			}
		}

		out := entityMap[channel](raw)

		// Callbacks are one-shot, so the first subscriber
		// receives the message and the callback is deleted.
		found, ok := live.callbacks.LoadAndDelete(channel)

		if ok && found != nil {
			callback, ok := found.(Callback[any])

			if ok {
				callback.Send(out)
			}
		}

		subscribers, ok := live.subscribers.Load(channel)

		if ok || subscribers != nil {
			for _, subscriber := range subscribers.([]types.Subscription[any]) {
				subscriber.Send(out)
			}
		}
	})

	live.client.OnConnected.Recurring(func(event *callback.Event[any]) {
		live.status = types.PENDING
		live.authenticate()
	})

	if auth {
		live.client.OnAuthenticated.Recurring(func(event *callback.Event[string]) {
			live.status = types.READY
		})
	}

	return live
}

func (live *Live) Status() types.Status {
	return live.status
}

func (live *Live) authenticate() (err error) {
	if live.nonceErr != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: auth nonce unavailable",
			live.nonceErr,
		))
	}

	if err = live.client.Authenticate(); err != nil && !strings.Contains(
		err.Error(), "Invalid nonce",
	) {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: authentication failed",
			err,
		))
	}

	if err == nil {
		return nil
	}

	if live.nonce != nil {
		live.nonce.Bump()
	}

	return live.client.Authenticate()
}

func (live *Live) Initialize() error {
	errnie.Info("initializing live")
	live.status = types.INITIALIZING

	if err := live.client.Connect(); err != nil {
		live.status = types.ERROR

		return errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: connect failed",
			err,
		))
	}

	return nil
}

func (live *Live) Subscribe(
	key string, subscription *types.Subscription[any],
) *types.Subscription[any] {
	subscribers, ok := live.subscribers.LoadOrStore(
		key, subscription,
	)

	if ok {
		subscribers = append(
			subscribers.([]*types.Subscription[any]), subscription,
		)

		live.subscribers.Store(key, subscribers)
	}

	return subscription
}

func (live *Live) Client() *spot.WebSocket {
	return live.client
}

func (live *Live) SubInstrument(callback types.Subscription[any]) {
	live.Write(kraken.NewInstrumentSubscription(), Callback[any]{
		Channel:      "instrument",
		Subscription: callback,
	})
}

func (live *Live) SubTicker(symbols []string)  { live.client.SubTicker(symbols) }
func (live *Live) SubBook(symbols []string)    { live.client.SubBook(symbols, 10) }
func (live *Live) SubTrades(symbols []string)  { live.client.SubTrades(symbols) }
func (live *Live) SubL3(symbols []string)      { live.client.SubL3(symbols, 10) }
func (live *Live) SubCandles(symbols []string) { live.client.SubCandles(symbols) }

func (live *Live) Books() map[string]*book.Book {
	return live.book.All()
}

func (live *Live) Book(symbol string) *book.Book {
	return live.book.Get(symbol)
}

func (live *Live) Balance() (map[string]*decimal.Decimal, error) {
	if live.model == "real" {
		response, err := live.client.REST.Balances()

		return response.Result, errnie.Error(errnie.Err(
			errnie.IO,
			"balance: failed to fetch",
			err,
		))
	}

	return live.paper.Balances()
}

func (live *Live) TradeBalance() (spot.TradesHistoryResult, error) {
	if live.model == "real" {
		response, err := live.client.REST.TradesHistory(
			&spot.TradesHistoryRequest{
				Type:             "all",
				Trades:           true,
				Start:            0,
				End:              0,
				Ofs:              0,
				ConsolidateTaker: true,
				Ledgers:          true,
			},
		)

		return response.Result, errnie.Error(errnie.Err(
			errnie.IO,
			"trade balance: failed to fetch",
			err,
		))
	}

	return live.paper.TradeBalance()
}

func (live *Live) TradeVolume(symbols []string) (*kraken.TradeVolumeResult, error) {
	response, err := live.Post(
		TradeVolumeEndpoint,
		kraken.NewTradeVolumeRequest(symbols),
	)

	return kraken.NewTradeVolume(response), errnie.Error(errnie.Err(
		errnie.IO,
		"trade volume: failed to fetch",
		err,
	))
}

func (live *Live) AddOrder(order *spot.AddOrderRequest) (spot.AddOrderResult, error) {
	if live.model == "paper" {
		response, err := live.client.REST.AddOrder(order)

		return response.Result, errnie.Error(errnie.Err(
			errnie.IO,
			"add order: failed to submit",
			err,
		))
	}

	return live.paper.AddOrder(order)
}

func (live *Live) Write(params json.Marshaler, callbacks ...Callback[any]) error {
	for _, callback := range callbacks {
		live.callbacks.Store(callback.Channel, callback.Subscription)
	}

	raw, err := params.MarshalJSON()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: write marshal failed",
			err,
		))
	}

	started := time.Now()

	err = live.client.WriteMessage(
		gorillawebsocket.TextMessage, raw,
	)

	if live.simulator != nil {
		live.simulator.Record(WEBSOCKET, time.Since(started))
	}

	return errnie.Error(err)
}

func (live *Live) do(options spot.RequestOptions) ([]byte, error) {
	started := time.Now()

	request, err := live.client.REST.NewRequest(options)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			err.Error(),
			err,
		))
	}

	resp, err := request.Do()

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"Kraken REST request failed",
			err,
		))
	}

	errors := utils.GetStringSlice(resp.Body, "error")

	if len(errors) > 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			errors[0],
			nil,
		))
	}

	if resp.StatusCode != 200 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"websocket.Live.do[%d]: %s",
				resp.StatusCode,
				resp.Body,
			),
			nil,
		))
	}

	if live.simulator != nil {
		live.simulator.Record(REST, time.Since(started))
	}

	return resp.Body, nil
}

func (live *Live) Post(
	path string, params json.Marshaler,
) ([]byte, error) {
	return live.do(spot.RequestOptions{
		Auth:   live.auth,
		Path:   path,
		Method: "POST",
		Body:   params,
	})
}

func (live *Live) Close() {
	live.cancel()

	if live.client.IsActive() {
		errnie.Error(live.client.Disconnect())
	}
}
