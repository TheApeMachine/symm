package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
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
	"ohlc":       func(buf []byte) any { return kraken.NewOHLC(buf) },
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
	quote       string
	simulator   *Simulator
	normalizer  *spot.Normalizer
	level3      *sync.Map
	book        *Book
	bookUpdates chan string
	symbols     []string
	publicMu    sync.RWMutex
	public      map[string][][]string
	auth        bool
	nonce       *AuthNonce
	nonceErr    error
	subscribers *sync.Map
	callbacks   *sync.Map
	priceIncr   sync.Map
	resyncing   sync.Map
	paper       *Paper
	model       string

	// Level3Client supplies the websocket client for the child connections
	// SubL3 opens. When nil the child dials the real Level3 endpoint; tests
	// set it to keep those children on the fixture transport.
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
		client.URL = endpoint
	}

	ctx, cancel := context.WithCancel(ctx)

	viper.SetDefault("market.quote_currency", "USD")

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
		public:      make(map[string][][]string),
		bookUpdates: make(chan string, max(viper.GetInt("system.actor.buffer"), 1)),
		paper:       NewPaper(ctx, NewSimulator()),
		model:       viper.GetViper().GetString("trading.model"),
		quote:       viper.GetViper().GetString("market.quote_currency"),
	}

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
		live.book = NewBook(ctx, live.normalizer)
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

		if channel == "subscribe" {
			errMessage := utils.GetString(raw, "error")

			if errMessage != "" {
				errnie.Error(errnie.Err(
					errnie.IO,
					"websocket: subscription rejected: "+errMessage,
					nil,
				))
				live.statusMu.Lock()
				live.status = types.ERROR
				live.statusMu.Unlock()
				return
			}
		}

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
		errnie.Info(fmt.Sprintf("websocket: connected to %s", live.client.URL))

		if auth {
			errnie.Error(live.authenticate())
			return
		}

		live.restorePublicSubscriptions()

		live.statusMu.Lock()
		live.status = types.READY
		live.statusMu.Unlock()
	})

	live.client.OnDisconnected.Recurring(func(event *callback.Event[error]) {
		if gorillawebsocket.IsCloseError(
			event.Data,
			gorillawebsocket.CloseNormalClosure,
		) {
			return
		}

		errnie.Error(errnie.Err(
			errnie.Unauthorized,
			fmt.Sprintf("websocket %s disconnected: %s - %s", endpoint, event.Data.Error(), event.Data),
			event.Data,
		))

		live.statusMu.Lock()
		live.status = types.PENDING
		live.statusMu.Unlock()
	})

	if auth {
		live.client.OnAuthenticated.Recurring(func(event *callback.Event[string]) {
			errnie.Info(fmt.Sprintf("websocket: authenticated to %s", live.client.URL))

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

			if endpoint == Level3WebSocketURL {
				live.restoreLevel3Subscription()
			}

			live.statusMu.Lock()
			live.status = types.READY
			live.statusMu.Unlock()
		})
	}

	errnie.Info(fmt.Sprintf("websocket: connecting to %s", live.client.URL))
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

	if live.model != "real" &&
		(key == "balances" || key == "executions" || key == "add_order") {
		return live.paper.Subscribe(key, subscription)
	}

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

func (live *Live) SubTicker(symbols []string) {
	live.rememberPublicSubscription("ticker", symbols)
	errnie.Error(live.client.SubTicker(symbols))
}

func (live *Live) SubBook(symbols []string) {
	live.rememberPublicSubscription("book", symbols)
	errnie.Error(live.client.SubBook(symbols, 10))
}

func (live *Live) SubTrades(symbols []string) {
	live.rememberPublicSubscription("trade", symbols)
	errnie.Error(live.client.SubTrades(symbols))
}

func (live *Live) SubCandles(symbols []string) {
	live.rememberPublicSubscription("ohlc", symbols)
	errnie.Error(live.client.SubCandles(symbols))
}

func (live *Live) rememberPublicSubscription(channel string, symbols []string) {
	live.publicMu.Lock()
	defer live.publicMu.Unlock()

	batch := make([]string, 0, len(symbols))

	for _, candidate := range symbols {
		known := false

		for _, remembered := range live.public[channel] {
			if slices.Contains(remembered, candidate) {
				known = true
				break
			}
		}

		if !known {
			batch = append(batch, candidate)
		}
	}

	if len(batch) > 0 {
		live.public[channel] = append(live.public[channel], batch)
	}
}

func (live *Live) restorePublicSubscriptions() {
	live.publicMu.RLock()
	subscriptions := make(map[string][][]string, len(live.public))

	for channel, batches := range live.public {
		for _, batch := range batches {
			subscriptions[channel] = append(
				subscriptions[channel],
				append([]string{}, batch...),
			)
		}
	}
	live.publicMu.RUnlock()

	for channel, batches := range subscriptions {
		for _, batch := range batches {
			switch channel {
			case "ticker":
				errnie.Error(live.client.SubTicker(batch))
			case "book":
				errnie.Error(live.client.SubBook(batch, 10))
			case "trade":
				errnie.Error(live.client.SubTrades(batch))
			case "ohlc":
				errnie.Error(live.client.SubCandles(batch))
			}
		}
	}
}

func (live *Live) restoreLevel3Subscription() {
	if len(live.symbols) == 0 {
		return
	}

	for symbols := range slices.Chunk(live.symbols, 40) {
		errnie.Error(live.client.SubL3(symbols, viper.GetInt("market.l3_depth")))
	}
}

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
		conn.symbols = append([]string{}, groups...)
		conn.book.SetUpdates(live.bookUpdates)

		for group := range slices.Chunk(groups, 40) {
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
			conn.book.SnapshotInto(out)
		}

		return true
	})

	return out
}

func (live *Live) Book(symbol string, read func(*book.Book)) {
	if live.level3 == nil {
		read(nil)
		return
	}

	found := false
	live.level3.Range(func(_, value any) bool {
		conn, ok := value.(*Live)

		if !ok || conn.book == nil {
			return true
		}

		conn.book.Get(symbol, func(managed *book.Book) {
			found = true
			read(managed)
		})
		return !found
	})

	if !found {
		read(nil)
	}
}

/*
BookUpdates emits a symbol after its authoritative Level 3 cache applies an
update. Consumers use it to retry work that explicitly declared that symbol's
snapshot pending.
*/
func (live *Live) BookUpdates() <-chan string {
	return live.bookUpdates
}

func (live *Live) Balance() (map[string]*decimal.Decimal, error) {
	if live.model == "real" {
		response, err := live.client.REST.Balances()

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.IO,
				"balance: failed to fetch",
				err,
			))
		}

		return response.Result, nil
	}

	return live.paper.Balances()
}

func (live *Live) TradesHistory() (spot.TradesHistoryResult, error) {
	if live.model == "real" {
		result := spot.TradesHistoryResult{Trades: map[string]spot.Trade{}}
		offset := 0

		for {
			response, err := live.client.REST.TradesHistory(&spot.TradesHistoryRequest{
				Type:             "all",
				Trades:           true,
				Start:            0,
				End:              0,
				Ofs:              offset,
				ConsolidateTaker: true,
				Ledgers:          true,
			})

			if err != nil {
				return spot.TradesHistoryResult{}, errnie.Error(err)
			}

			maps.Copy(result.Trades, response.Result.Trades)

			result.Count = response.Result.Count

			count, err := strconv.Atoi(response.Result.Count.String())

			if err == nil && len(result.Trades) >= count {
				return result, nil
			}

			if len(response.Result.Trades) == 0 {
				return result, nil
			}

			offset += len(response.Result.Trades)
		}
	}

	return live.paper.TradesHistory()
}

func (live *Live) OpenOrders() (spot.OpenOrdersResult, error) {
	if live.model != "real" {
		return live.paper.OpenOrders()
	}

	response, err := live.client.REST.OpenOrders(&spot.OpenOrdersRequest{Trades: true})

	if err != nil {
		return spot.OpenOrdersResult{}, errnie.Error(err)
	}

	return response.Result, nil
}

func (live *Live) CancelOrder(
	request *spot.CancelOrderRequest,
) (spot.CancelResult, error) {
	if live.model != "real" {
		return live.paper.CancelOrder(request)
	}

	response, err := live.client.REST.CancelOrder(request)

	if err != nil {
		return spot.CancelResult{}, errnie.Error(err)
	}

	return response.Result, nil
}

func (live *Live) TradeBalance() (kraken.TradeBalanceResult, error) {
	if live.model == "real" {
		response, err := live.Post(
			TradeBalanceEndpoint,
			kraken.NewTradeBalanceRequest(live.quote),
		)

		return kraken.NewTradeBalance(response), errnie.Error(err)
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
	// Only a real model reaches the venue. The test read the other way round,
	// which sent paper orders to Kraken over REST and routed real ones into
	// the simulator.
	if live.model == "real" {
		response, err := live.client.REST.AddOrder(order)

		if err != nil {
			return spot.AddOrderResult{}, errnie.Error(errnie.Err(
				errnie.IO,
				"add order: failed to submit",
				err,
			))
		}

		return response.Result, nil
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
