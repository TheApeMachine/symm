package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	gorillawebsocket "github.com/gorilla/websocket"
	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

const (
	PublicWebSocketURL  = "wss://ws.kraken.com/v2"
	PrivateWebSocketURL = "wss://ws-auth.kraken.com/v2"
	Level3WebSocketURL  = "wss://ws-l3.kraken.com/v2"
)

/*
Live is the spot websocket and REST transport.
*/
type Live struct {
	status       atomic.Value
	ctx          context.Context
	cancel       context.CancelFunc
	client       *spot.WebSocket
	sync         *sync.Map
	paper        *Paper
	simulator    *Simulator
	auth         bool
	books        *spot.BookManager
	bookMu       sync.RWMutex
	isLevel3     bool
	symbols      []string
	reconnectMu  sync.Mutex
	reconnectFns []func()
}

/*
New opens a spot websocket transport.
*/
func New(
	ctx context.Context,
	simulator *Simulator,
	auth bool,
	endpoint string,
) *Live {
	ctx, cancel := context.WithCancel(ctx)

	live := &Live{
		ctx:       ctx,
		cancel:    cancel,
		simulator: simulator,
		client:    spot.NewWebSocket(),
		sync:      &sync.Map{},
		auth:      auth,
	}
	live.status.Store(types.INITIALIZING)

	live.client.URL = endpoint

	if endpoint == Level3WebSocketURL {
		live.isLevel3 = true
	}

	if live.auth {
		live.client.REST.PublicKey = os.Getenv("KRAKEN_API_KEY")
		live.client.REST.PrivateKey = os.Getenv("KRAKEN_API_SECRET")
		// Private and every Level3 batch authenticate with the same key; they
		// must share one monotonic nonce sequence or concurrent token fetches
		// collide (EAPI:Invalid nonce).
		live.client.REST.Nonce = authNonce()
	}

	if live.isLevel3 {
		live.books = spot.NewBookManager()
		live.books.OnCreateBook.Recurring(func(event *callback.Event[*book.Book]) {
			managed := event.Data

			// Kraken frames are atomic, so depth cannot be enforced per order.
			managed.EnableMaxDepth = false
			managed.NoBookCrossing = false
			managed.OnChecksummed.Recurring(
				func(*callback.Event[*book.ChecksumResult]) {
					managed.EnforceDepth()
				},
			)
		})
	}

	live.client.OnSent.Recurring(func(event *callback.Event[*kraken.WebSocketMessage]) {
		if err := live.updateLevel3(event); err != nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"websocket: level3 book update failed: "+err.Error(),
				err,
			))
		}
	})

	live.client.OnReceived.Recurring(func(event *callback.Event[*kraken.WebSocketMessage]) {
		if err := live.updateLevel3(event); err != nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"websocket: level3 "+utils.GetString(
					event.Data.Bytes(), "type",
				)+" failed: "+err.Error(),
				err,
			))
		}

		live.route(event.Data.Bytes())
	})

	live.client.OnConnected.Recurring(func(event *callback.Event[any]) {
		if !live.auth {
			live.fireReconnect()
			live.status.Store(types.READY)
			return
		}

		if errnie.Error(live.authenticate()) != nil {
			live.status.Store(types.ERROR)
		}
	})

	if live.auth {
		live.client.OnAuthenticated.Recurring(func(event *callback.Event[string]) {
			if live.isLevel3 && len(live.symbols) > 0 && live.SubscribeLevel3(
				live.symbols,
				viper.GetInt("market.l3_depth"),
			) != nil {
				errnie.Error(errnie.Err(
					errnie.Validation,
					"websocket: level3 book subscription failed",
					nil,
				))
			}

			live.fireReconnect()
			live.status.Store(types.READY)
		})
	}

	return live
}

/*
authenticate fetches a websocket token. An Invalid nonce rejection bumps the
persisted high-water and retries once so reconnect storms after a crash do not
leave the transport permanently in ERROR.
*/
func (live *Live) authenticate() error {
	err := live.client.Authenticate()

	if err == nil {
		return nil
	}

	if !strings.Contains(err.Error(), "Invalid nonce") {
		return err
	}

	bumpAuthNonce()

	return live.client.Authenticate()
}

/*
OnReconnect registers a callback invoked after public connect or private
authentication so subscription intent can be replayed.
*/
func (live *Live) OnReconnect(fn func()) {
	if live == nil || fn == nil {
		return
	}

	live.reconnectMu.Lock()
	live.reconnectFns = append(live.reconnectFns, fn)
	live.reconnectMu.Unlock()
}

func (live *Live) fireReconnect() {
	live.reconnectMu.Lock()
	callbacks := append([]func(){}, live.reconnectFns...)
	live.reconnectMu.Unlock()

	for _, callback := range callbacks {
		callback()
	}
}

/*
SubscribeLevel3 sends the configured depth explicitly because the SDK's depth
argument is not included in its level3 subscription payload.
*/
func (live *Live) SubscribeLevel3(symbols []string, depth int) error {
	return live.client.SubPrivate("level3", map[string]any{
		"params": map[string]any{
			"symbol": symbols,
			"depth":  depth,
		},
	})
}

/*
updateLevel3 applies one complete websocket message before truncating affected
books, preserving Kraken's atomic L3 message boundary. The write lease excludes
PeekBook readers so Side.Levels is never ranged while the SDK mutates it.
*/
func (live *Live) updateLevel3(
	event *callback.Event[*kraken.WebSocketMessage],
) error {
	if !live.isLevel3 || live.books == nil {
		return nil
	}

	live.bookMu.Lock()
	defer live.bookMu.Unlock()

	return live.books.Update(event)
}

/*
peekBook calls fn while holding the Level3 read lease for this transport.
*/
func (live *Live) peekBook(symbol string, fn func(*book.Book)) bool {
	if live == nil || live.books == nil || fn == nil || symbol == "" {
		return false
	}

	live.bookMu.RLock()
	defer live.bookMu.RUnlock()

	symbolBook := live.books.GetBook(symbol)

	if symbolBook == nil {
		return false
	}

	fn(symbolBook)

	return true
}

/*
ApplyLevel3 feeds one raw Level3 websocket payload through the write lease.
*/
func (live *Live) ApplyLevel3(payload []byte) error {
	if live == nil {
		return nil
	}

	if len(payload) == 0 {
		return errnie.Err(
			errnie.Validation,
			"websocket: level3 payload is empty",
			nil,
		)
	}

	return live.updateLevel3(&callback.Event[*kraken.WebSocketMessage]{
		Data: kraken.NewWebSocketMessage(payload),
	})
}

/*
SeedTouchDecimals installs a two-sided L3 touch using exact decimal prices.
*/
func (live *Live) SeedTouchDecimals(
	symbol string,
	bid *decimal.Decimal,
	ask *decimal.Decimal,
	quantity float64,
	at time.Time,
) {
	if live == nil || live.books == nil || symbol == "" || bid == nil || ask == nil {
		return
	}

	live.bookMu.Lock()
	defer live.bookMu.Unlock()

	symbolBook := live.books.GetBook(symbol)

	if symbolBook == nil {
		symbolBook = live.books.CreateBook(symbol, 10)
		symbolBook.EnableMaxDepth = false
		symbolBook.NoBookCrossing = false
	}

	quantityDecimal := decimal.NewFromFloat64(quantity)
	symbolBook.Update(&book.UpdateOptions{
		Direction: book.Bid,
		ID:        "seed-bid",
		Price:     bid,
		Quantity:  quantityDecimal,
		Timestamp: at,
	})
	symbolBook.Update(&book.UpdateOptions{
		Direction: book.Ask,
		ID:        "seed-ask",
		Price:     ask,
		Quantity:  quantityDecimal,
		Timestamp: at,
	})
}

/*
SeedTouch installs a two-sided L3 touch for symbol under the write lease so
toxicity harness tests can PeekBook without checksummed fixture replay.
*/
func (live *Live) SeedTouch(
	symbol string,
	bid float64,
	ask float64,
	quantity float64,
	at time.Time,
) {
	if live == nil || live.books == nil || symbol == "" {
		return
	}

	live.SeedTouchDecimals(
		symbol,
		decimal.NewFromFloat64(bid),
		decimal.NewFromFloat64(ask),
		quantity,
		at,
	)
}

func (live *Live) Initialize() error {
	errnie.Info("initializing live")

	if err := live.client.Connect(); err != nil {
		live.status.Store(types.ERROR)

		return errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: connect failed",
			err,
		))
	}

	live.status.Store(types.READY)
	return nil
}

func (live *Live) route(raw []byte) {
	channel := utils.GetString(raw, "channel")

	if channel == "" {
		if method := utils.GetString(raw, "method"); method == "add_order" {
			channel = method
		}
	}

	if channel == "" {
		if message := utils.GetString(raw, "error"); message != "" {
			errnie.Error(errnie.Err(errnie.Validation, message, nil))
		}

		return
	}

	if channel == "status" || channel == "heartbeat" {
		return
	}

	if live.isLevel3 && channel == "level3" {
		return
	}

	live.dispatch(channel, raw)
}

func (live *Live) dispatch(channel string, raw []byte) {
	callbacks, ok := live.sync.Load(channel)

	if !ok {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: channel "+channel+" not found",
			nil,
		))

		return
	}

	for _, cb := range callbacks.([]func([]byte)) {
		cb(raw)
	}
}

func (live *Live) Status() types.Status {
	status := live.status.Load()

	if status == nil {
		return types.INITIALIZING
	}

	return status.(types.Status)
}

func (live *Live) Client() *spot.WebSocket {
	return live.client
}

func (live *Live) On(
	channel string, action func([]byte),
) {
	callbacks, ok := live.sync.LoadOrStore(channel, []func([]byte){action})

	if ok {
		callbacks = append(callbacks.([]func([]byte)), action)
		live.sync.Store(channel, callbacks)
	}
}

func (live *Live) Write(params json.Marshaler) error {
	raw, err := params.MarshalJSON()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: write marshal failed",
			err,
		))
	}

	methodNode, err := sonic.Get(raw, "method")

	if err != nil || !methodNode.Exists() {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			err.Error(),
			err,
		))
	}

	started := time.Now()

	writeErr := live.client.WriteMessage(
		gorillawebsocket.TextMessage, raw,
	)

	if live.simulator != nil {
		live.simulator.Record(WEBSOCKET, time.Since(started))
	}

	return errnie.Error(writeErr)
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

func (live *Live) Get(
	path string, params json.Marshaler,
) ([]byte, error) {
	return live.do(spot.RequestOptions{
		Auth:   live.auth,
		Path:   path,
		Method: "GET",
		Query:  params,
	})
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
	if live.paper != nil {
		live.paper.Close()
	}

	live.cancel()

	if live.client.IsActive() {
		errnie.Error(live.client.Disconnect())
	}
}
