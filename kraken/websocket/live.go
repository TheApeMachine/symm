package websocket

import (
	"context"
	"encoding/json"
	"fmt"
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
	"balances":   func(buf []byte) any { return kraken.NewBalance(buf) },
	"executions": func(buf []byte) any { return kraken.NewExecution(buf) },
	"status":     func(buf []byte) any { return true },
	"heartbeat":  func(buf []byte) any { return true },
	"subscribe":  func(buf []byte) any { return true },
	"pong": func(buf []byte) any {
		pong := map[string]any{}
		errnie.Error(sonic.Unmarshal(buf, &pong))
		return pong
	},
}

/*
Live is one spot websocket session: SDK client, channel fan-out, auth/nonce,
and Sub* resubscribe after the SDK reconnects.
*/
type Live struct {
	status      types.Status
	statusMu    sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
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

	/*
		Level3Client supplies the websocket client for the child connections
		SubL3 opens. When nil the child dials the real Level3 endpoint; tests
		set it to keep those children on the fixture transport.
	*/
	Level3Client func() *spot.WebSocket
}

/*
newLevel3 opens a child connection for a Level3 symbol group, honouring an
injected client when one is configured.
*/
func (live *Live) newLevel3(
	ctx context.Context,
	simulator *Simulator,
	auth bool,
	endpoint string,
) *Live {
	var client *spot.WebSocket

	if live.Level3Client != nil {
		client = live.Level3Client()
	}

	child := NewWithClient(ctx, simulator, auth, endpoint, client)

	if child != nil {
		child.Level3Client = live.Level3Client
	}

	return child
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
	return NewWithClient(ctx, simulator, auth, endpoint, nil)
}

/*
NewWithClient opens a spot websocket session using an injected spot.WebSocket client instance.
This allows simulation and tests to supply a mock WebSocket client as the data source while
retaining full production parsing, event routing, and book handling in Live.
*/
func NewWithClient(
	ctx context.Context,
	simulator *Simulator,
	auth bool,
	endpoint string,
	client *spot.WebSocket,
) *Live {
	if client == nil {
		client = spot.NewWebSocket()
	}

	ctx, cancel := context.WithCancel(ctx)

	live := &Live{
		ctx:         ctx,
		cancel:      cancel,
		status:      types.INITIALIZING,
		simulator:   simulator,
		client:      client,
		normalizer:  spot.NewNormalizer(),
		auth:        auth,
		subscribers: &sync.Map{},
		callbacks:   &sync.Map{},
		paper:       NewPaper(ctx, NewSimulator()),
		model:       viper.GetViper().GetString("trading.model"),
	}

	live.client.URL = endpoint

	if err := live.normalizer.Use(live.client.REST); err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: failed to initialize normalizer",
			err,
		))

		cancel()
		return nil
	}

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

		handler, ok := entityMap[channel]

		if !ok {
			errnie.Error(errnie.Err(
				errnie.NotFound,
				"websocket: unhandled channel "+channel,
				nil,
			))

			return
		}

		out := handler(raw)

		if channel == "level3" && live.book != nil {
			level3, ok := out.(*kraken.Level3)

			if !ok {
				errnie.Error(errnie.Err(
					errnie.Validation,
					"websocket: unexpected level3 payload type",
					nil,
				))

				return
			}

			errnie.Error(live.book.Update(event, level3))
			return
		}

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

		if auth {
			errnie.Error(live.authenticate())
			return
		}

		live.statusMu.Lock()
		live.status = types.READY
		live.statusMu.Unlock()
	})

	live.client.OnDisconnected.Recurring(func(event *callback.Event[error]) {
		errnie.Error(errnie.Err(
			errnie.Unauthorized,
			fmt.Sprintf("websocket %s disconnected: %s", endpoint, event.Data.Error()),
			event.Data,
		))

		live.statusMu.Lock()
		live.status = types.PENDING
		live.statusMu.Unlock()
	})

	if auth {
		live.client.OnAuthenticated.Recurring(func(event *callback.Event[string]) {
			errnie.Info(fmt.Sprintf("websocket: authenticated to %s", endpoint))

			if endpoint == PrivateWebSocketURL {
				err := live.subscribeAccount(event.Data)

				if err != nil {
					errnie.Error(errnie.Err(
						errnie.IO,
						"websocket: failed to subscribe to private account channels",
						err,
					))

					live.statusMu.Lock()
					live.status = types.ERROR
					live.statusMu.Unlock()
					return
				}
			}

			live.statusMu.Lock()
			live.status = types.READY
			live.statusMu.Unlock()
		})
	}

	errnie.Info(fmt.Sprintf("websocket: connecting to %s", endpoint))
	live.statusMu.Lock()
	live.status = types.PENDING
	live.statusMu.Unlock()

	if err := live.client.Connect(); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"websocket: failed to connect",
			err,
		))
	}

	return live
}

func (live *Live) Status() types.Status {
	live.statusMu.RLock()
	defer live.statusMu.RUnlock()

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

/*
subscribeAccount activates Kraken's private wallet and execution streams after
each token refresh. Kraken closes an authenticated socket that does not submit
a private subscription within its token deadline, so reconnect authentication
must always repeat these requests with the new token.
*/
func (live *Live) subscribeAccount(token string) error {
	if err := live.Write(kraken.NewBalanceSubscription(token)); err != nil {
		return err
	}

	return live.Write(kraken.NewExecutionSubscription(token))
}

func (live *Live) Subscribe(
	key string, subscription *types.Subscription[any],
) *types.Subscription[any] {
	errnie.Info(fmt.Sprintf("websocket: new subscriber %s", key))

	return utils.Subscribe(
		live.subscribers, key, subscription,
	)
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
		conn := live.newLevel3(
			live.ctx, live.simulator, live.auth, Level3WebSocketURL,
		)

		if conn == nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"websocket: failed to create level3 child connection",
				nil,
			))

			continue
		}

		groupKey := strings.Join(groups, "|")
		live.level3.Store(groupKey, conn)

		for group := range slices.Chunk(groups, 40) {
			errnie.Info(fmt.Sprintf(
				"websocket: subscribing to level3 %s",
				strings.Join(group, "|"),
			))

			if conn.book != nil {
				for _, symbol := range group {
					conn.book.Create(symbol, viper.GetInt("market.l3_depth"))
				}
			}

			conn.Client().SubL3(group, viper.GetInt("market.l3_depth"))
			time.Sleep(viper.GetDuration("market.subscribe_pace"))
		}
	}
}

func (live *Live) Books() *sync.Map {
	out := &sync.Map{}

	if live.level3 == nil {
		return out
	}

	live.level3.Range(func(key, value any) bool {
		if conn, ok := value.(*Live); ok && conn.book != nil {
			conn.book.All().Range(func(symbol, book any) bool {
				out.Store(symbol, book)
				return true
			})
		}

		return true
	})

	return out
}

func (live *Live) Book(symbol string) *book.Book {
	var manager *book.Book

	if live.level3 == nil {
		return nil
	}

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
