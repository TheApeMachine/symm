package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	gorillawebsocket "github.com/gorilla/websocket"
	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	sdkkraken "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
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
	"subscribe":  func(buf []byte) any { return func() bool { return true }() },
	"pong": func(buf []byte) any {
		return func() map[string]any {
			pong := map[string]any{}
			errnie.Error(sonic.Unmarshal(buf, &pong))
			return pong
		}()
	},
}

/*
Live is one spot websocket session: SDK client, channel fan-out, auth/nonce,
and Sub* resubscribe after the SDK reconnects.
*/
type Live struct {
	status      types.Status
	ctx         context.Context
	cancel      context.CancelFunc
	pingCtx     context.Context
	pingCancel  context.CancelFunc
	client      *spot.WebSocket
	simulator   *Simulator
	normalizer  *spot.Normalizer
	level3      *sync.Map
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
		normalizer:  spot.NewNormalizer(),
		auth:        auth,
		subscribers: &sync.Map{},
		callbacks:   &sync.Map{},
		paper:       NewPaper(ctx, NewSimulator()),
		model:       viper.GetViper().GetString("trading.model"),
	}

	live.client.URL = endpoint
	live.normalizer.Use(live.client.REST)

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
		live.level3 = &sync.Map{}
		live.book = NewBook(ctx)
	}

	live.client.OnReceived.Recurring(func(event *callback.Event[*sdkkraken.WebSocketMessage]) {
		raw := event.Data.Bytes()

		channel := utils.GetString(raw, "channel")

		if channel == "" {
			if method := utils.GetString(raw, "method"); method != "" {
				channel = method
			}
		}

		out := entityMap[channel](raw)

		if channel == "pong" {
			// Check the error field in the pong response. If it is not empty, log the error.
			if errMsg := utils.GetString(raw, "error"); errMsg != "" {
				errnie.Error(errnie.Err(
					errnie.IO,
					fmt.Sprintf("websocket: pong error: %s", errMsg),
					nil,
				))

				return
			}

			// If the pong response is successful, log it on the ping context.
			if live.pingCtx != nil {
				live.pingCtx = context.WithValue(live.pingCtx, "pong", time.Now())
			}

			return
		}

		// Callbacks are one-shot, so the first subscriber
		// receives the message and the callback is deleted.
		found, ok := live.callbacks.LoadAndDelete(channel)

		if ok && found != nil {
			subscription, ok := found.(types.Subscription[any])

			if ok {
				subscription.Send(out)
				close(subscription.Channel)
			}
		}

		subscribers, ok := live.subscribers.Load(channel)

		if ok && subscribers != nil {
			for _, subscriber := range subscribers.([]*types.Subscription[any]) {
				subscriber.Send(out)
			}
		}
	})

	live.client.OnConnected.Recurring(func(event *callback.Event[any]) {
		errnie.Info(fmt.Sprintf("websocket: connected to %s", endpoint))

		go func() {
			live.pingCtx, live.pingCancel = context.WithCancel(live.ctx)
			hasPing := live.pingCtx.Value("req_id")
			hasPong := live.pingCtx.Value("pong")

			if hasPing != nil && hasPong == nil {
				errnie.Error(errnie.Err(
					errnie.IO,
					"websocket: we ain't got no pong, yo!",
					nil,
				))
			}

			if hasPing == nil {
				live.pingCtx = context.WithValue(live.pingCtx, "req_id", 0)
				hasPing = 0
			}

			live.pingCtx = context.WithValue(live.pingCtx, "req_id", hasPing.(int)+1)
			live.pingCtx = context.WithValue(live.pingCtx, "pong", nil)

			pingRequest := datura.NewMap()
			pingRequest["method"] = "ping"
			pingRequest["req_id"] = live.pingCtx.Value("req_id")

			for {
				select {
				case <-live.pingCtx.Done():
					return
				case <-time.After(20 * time.Second):
					live.Write(pingRequest)
				}
			}
		}()

		if auth {
			errnie.Error(live.authenticate())
			return
		}

		live.status = types.READY
	})

	live.client.OnDisconnected.Recurring(func(event *callback.Event[error]) {
		errnie.Error(errnie.Err(
			errnie.Unauthorized,
			fmt.Sprintf("websocket %s disconnected: %s", endpoint, event.Data.Error()),
			event.Data,
		))

		live.status = types.PENDING
	})

	if auth {
		live.client.OnAuthenticated.Recurring(func(event *callback.Event[string]) {
			errnie.Info(fmt.Sprintf("websocket: authenticated to %s", endpoint))
			live.status = types.READY
		})
	}

	errnie.Info(fmt.Sprintf("websocket: connecting to %s", endpoint))
	live.status = types.PENDING
	live.client.Connect()

	return live
}

func (live *Live) Status() types.Status {
	return live.status
}

func (live *Live) authenticate() (err error) {
	errnie.Info(fmt.Sprintf("websocket[%s]: authenticating", live.client.URL))

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

func (live *Live) Subscribe(
	key string, subscription *types.Subscription[any],
) *types.Subscription[any] {
	errnie.Info(fmt.Sprintf("websocket: new subscriber %s", key))

	subscribers, ok := live.subscribers.LoadOrStore(
		key, []*types.Subscription[any]{subscription},
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
	errnie.Info("websocket: subscribing to instrument")

	live.Write(kraken.NewInstrumentSubscription(), Callback[any]{
		Channel:      "instrument",
		Subscription: callback,
	})
}

func (live *Live) SubTicker(symbols []string)  { live.client.SubTicker(symbols) }
func (live *Live) SubBook(symbols []string)    { live.client.SubBook(symbols, 10) }
func (live *Live) SubTrades(symbols []string)  { live.client.SubTrades(symbols) }
func (live *Live) SubCandles(symbols []string) { live.client.SubCandles(symbols) }

func (live *Live) SubL3(symbols []string) {
	if live.level3 == nil {
		live.level3 = &sync.Map{}
	}

	for groups := range slices.Chunk(symbols, 200) {
		conn := New(
			live.ctx, live.simulator, live.auth, Level3WebSocketURL,
		)

		groupKey := strings.Join(groups, "|")
		live.level3.Store(groupKey, conn)

		for group := range slices.Chunk(groups, 40) {
			errnie.Info(fmt.Sprintf(
				"websocket: subscribing to level3 %s",
				strings.Join(group, "|"),
			))

			conn.Client().SubL3(group, 10, map[string]any{
				"depth": 10,
			})

			time.Sleep(1 * time.Second)
		}
	}
}

func (live *Live) Books() map[string]*book.Book {
	out := map[string]*book.Book{}

	live.level3.Range(func(key, value any) bool {
		if conn, ok := value.(*Live); ok && conn.book != nil {
			maps.Copy(out, conn.book.All())
		}

		return true
	})

	return out
}

func (live *Live) Book(symbol string) *book.Book {
	var manager *book.Book

	live.level3.Range(func(key, value any) bool {
		keys := strings.Split(key.(string), "|")

		if slices.Contains(keys, symbol) {
			if conn, ok := value.(*Live); ok && conn.book != nil {
				manager = conn.book.Get(symbol)
				return false
			}
		}

		return true
	})

	return manager
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

		return response.Result, errnie.Error(err)
	}

	return live.paper.TradeBalance()
}

func (live *Live) TradeVolume(symbols []string) (*kraken.TradeVolumeResult, error) {
	response, err := live.Post(
		TradeVolumeEndpoint,
		kraken.NewTradeVolumeRequest(symbols),
	)

	return kraken.NewTradeVolume(response), errnie.Error(err)
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
