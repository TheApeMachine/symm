package websocket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/theapemachine/symm/nomagique/runtime"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	publicBookDepth     = 10
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
	status       atomic.Pointer[types.Status]
	ctx          context.Context
	cancel       context.CancelFunc
	client       *spot.WebSocket
	quote        string
	thesis       atomic.Pointer[types.Thesis]
	bus          atomic.Pointer[runtime.Workspace]
	tickersCh    *runtime.Channel[kraken.TickerData]
	tradesCh     *runtime.Channel[kraken.TradeData]
	level3Ch     *runtime.Channel[kraken.Level3Data]
	executionsCh *runtime.Channel[kraken.ExecutionData]
	uiCh         *runtime.Channel[*types.UIFrame]
	simulator    *Simulator
	normalizer   *spot.Normalizer
	level3       *sync.Map
	book         *Book
	symbols      []string
	publicMu     sync.RWMutex
	public       map[string][][]string
	auth         bool
	nonce        *AuthNonce
	nonceErr     error
	subscribers  *sync.Map
	callbacks    *sync.Map
	priceIncr    sync.Map
	paper        *Paper
	model        string
	capture      CaptureSink
	captureName  string
	failureMu    sync.RWMutex
	failure      func(error)
	observer     atomic.Pointer[func(string, time.Duration)]

	// level3Client overrides the venue client SubL3 dials when set. Fixtures
	// inject the level3 listener's client here so a replay's level3 frames
	// feed the session's book manager instead of dialing the real venue.
	level3Client func() *spot.WebSocket
}

/*
Capture returns the underlying capture sink attached to the live connection.
*/
func (live *Live) Capture() CaptureSink {
	if live == nil {
		return nil
	}

	return live.capture
}

/*
SetObserver attaches the ingress processing clock. Existing and future Level 3
children share the same observer so all venue messages contribute to the one
ingress stage shown by diagnostics.
*/
func (live *Live) SetObserver(observer func(string, time.Duration)) {
	if live == nil {
		return
	}

	if observer == nil {
		live.observer.Store(nil)
	} else {
		live.observer.Store(&observer)
	}

	if live.level3 == nil {
		return
	}

	live.level3.Range(func(_, value any) bool {
		if child, valid := value.(*Live); valid && child != nil {
			child.SetObserver(observer)
		}

		return true
	})
}

/*
SetFailureHandler connects fatal ingestion failures to the transport owner.
Level 3 child sessions inherit the same handler when they are attached.
*/
func (live *Live) SetFailureHandler(handler func(error)) {
	if live == nil {
		return
	}

	live.failureMu.Lock()
	live.failure = handler
	live.failureMu.Unlock()

	if live.level3 == nil {
		return
	}

	live.level3.Range(func(_, value any) bool {
		if child, valid := value.(*Live); valid && child != nil {
			child.SetFailureHandler(handler)
		}

		return true
	})
}

func (live *Live) reportFailure(err error) {
	if live == nil || err == nil {
		return
	}

	live.setStatus(types.ERROR)
	live.failureMu.RLock()
	handler := live.failure
	live.failureMu.RUnlock()

	if handler != nil {
		handler(err)
	}
}

/*
SetThesis attaches the canonical event destination before subscriptions begin
producing market frames.
*/
func (live *Live) SetThesis(thesis *types.Thesis) {
	if live == nil || thesis == nil {
		panic("websocket: thesis required")
	}

	live.thesis.Store(thesis)

	if live.paper != nil {
		live.paper.SetThesis(thesis)
	}
}

/*
SetBus attaches the system workspace bus and resolves the ingest channels.
*/
func (live *Live) SetBus(bus *runtime.Workspace) {
	if live == nil || bus == nil {
		return
	}

	live.bus.Store(bus)

	if live.book != nil {
		live.book.SetBus(bus)
	}

	live.tickersCh = runtime.ChannelOf(
		bus, types.ChannelTickers,
		func(ticker kraken.TickerData) string { return ticker.Symbol },
	)
	live.tradesCh = runtime.ChannelOf(
		bus, types.ChannelTrades,
		func(trade kraken.TradeData) string { return trade.Symbol },
	)
	live.level3Ch = runtime.ChannelOf(
		bus, types.ChannelLevel3,
		func(frame kraken.Level3Data) string { return frame.Symbol },
	)
	live.executionsCh = runtime.ChannelOf(
		bus, types.ChannelExecutions,
		func(execution kraken.ExecutionData) string { return execution.Symbol },
	)
	live.uiCh = runtime.ChannelOf(
		bus, types.ChannelUI,
		func(frame *types.UIFrame) string { return "" },
	)
}

/*
New opens a spot websocket session and wires SDK callbacks in the constructor.
*/
func New(
	ctx context.Context,
	thesis *types.Thesis,
	simulator *Simulator,
	auth bool,
	endpoint string,
	recorders ...CaptureSink,
) *Live {
	return NewWithClient(ctx, thesis, simulator, auth, endpoint, nil, recorders...)
}

/*
NewWithClient opens a spot websocket session using an injected spot.WebSocket client instance.
A nil Thesis creates an explicit parsing-only session; SetThesis attaches event routing before
the connection becomes part of a running system.
*/
func NewWithClient(
	ctx context.Context,
	thesis *types.Thesis,
	simulator *Simulator,
	auth bool,
	endpoint string,
	client *spot.WebSocket,
	recorders ...CaptureSink,
) *Live {
	if len(recorders) > 1 {
		panic("websocket: at most one market capture sink is supported")
	}

	if client == nil {
		client = spot.NewWebSocket()
		client.URL = endpoint
	}

	ctx, cancel := context.WithCancel(ctx)

	viper.SetDefault("market.quote_currency", "USD")

	captureName := "public"

	if auth {
		captureName = "private"
	}

	if endpoint == Level3WebSocketURL {
		captureName = "level3"
	}

	live := &Live{
		ctx:         ctx,
		cancel:      cancel,
		simulator:   simulator,
		client:      client,
		normalizer:  spot.NewNormalizer(),
		auth:        auth,
		subscribers: &sync.Map{},
		callbacks:   &sync.Map{},
		public:      make(map[string][][]string),
		paper:       NewPaper(ctx, NewSimulator(), thesis),
		model:       viper.GetViper().GetString("trading.model"),
		quote:       viper.GetViper().GetString("market.quote_currency"),
		captureName: captureName,
	}

	if thesis != nil {
		live.SetThesis(thesis)
	}

	live.setStatus(types.INITIALIZING)

	if len(recorders) == 1 {
		live.capture = recorders[0]
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
		live.book.SetBus(live.bus.Load())
		live.book.emit = func(data kraken.Level3Data) {
			thesis := live.thesis.Load()

			if thesis != nil && thesis.Symbol(data.Symbol).AcceptLevel3(data.Timestamp) && live.level3Ch != nil {
				live.level3Ch.Publish(data)
			}
		}
		// A checksum divergence recovers by re-running the same whole-universe
		// level3 subscribe the startup path uses, on this child's already
		// connected socket. book.symbols is filled once the parent assigns the
		// group, so the closure reads it lazily at recovery time.
		live.book.resubscribe = func() {
			live.subscribeLevel3Group(live)
			live.book.resyncDone()
		}
	}

	live.client.OnReceived.Recurring(func(event *callback.Event[*sdkkraken.WebSocketMessage]) {
		observer := live.observer.Load()

		if observer != nil {
			started := time.Now()
			defer func() {
				(*observer)("crypto", time.Since(started))
			}()
		}

		raw := event.Data.Bytes()

		if err := live.captureFrame(live.captureName, raw); err != nil {
			if _, ok := errors.AsType[types.SaturatedError](err); !ok {
				errnie.Error(errnie.Err(
					errnie.IO,
					"websocket: market capture failed",
					err,
				))
			}
		}

		channel := utils.GetString(raw, "channel")

		if channel == "" {
			if method := utils.GetString(raw, "method"); method != "" {
				channel = method
			}
		}

		// An unsubscribe acknowledgement answers the instrument's paced
		// recovery of a checksum-diverged symbol; there is nothing to
		// dispatch for it.
		if channel == "unsubscribe" {
			return
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
				live.reportFailure(errnie.Err(
					errnie.Validation,
					"websocket: unexpected level3 payload type",
					nil,
				))
				return
			}

			if err := live.book.Update(event, level3); err != nil {
				live.reportFailure(err)
				return
			}

			return
		}

		if channel == "subscribe" {
			errMessage := utils.GetString(raw, "error")

			if errMessage != "" {
				errnie.Error(errnie.Err(
					errnie.IO,
					"websocket: subscription rejected: "+errMessage,
					nil,
				))
				live.setStatus(types.ERROR)
				return
			}
		}

		switch entity := out.(type) {
		case *kraken.Ticker:
			thesis := live.thesis.Load()

			if thesis != nil {
				for index := range entity.Data {
					ticker := entity.Data[index]
					tick := thesis.AdvanceTick(ticker.Timestamp)
					symbol := thesis.Symbol(ticker.Symbol)
					symbol.Tick = tick

					if symbol.AcceptTicker(ticker.Timestamp) && live.tickersCh != nil {
						live.tickersCh.Publish(ticker)
					}

					if live.uiCh != nil {
						live.uiCh.Publish(&types.UIFrame{
							Type: wire.FrameTickFrame,
							Value: &wire.TickFrameT{
								Count: tick,
							},
						})
					}
				}
			}
		case *kraken.Trade:
			thesis := live.thesis.Load()

			if thesis != nil {
				for index := range entity.Data {
					trade := entity.Data[index]

					if thesis.Symbol(trade.Symbol).AcceptTrade(trade.Timestamp) && live.tradesCh != nil {
						live.tradesCh.Publish(trade)
					}
				}
			}
		case *kraken.Execution:
			thesis := live.thesis.Load()

			if thesis != nil {
				for index := range entity.Data {
					execution := entity.Data[index]

					if live.executionsCh != nil {
						live.executionsCh.Publish(execution)
					}
				}
			}
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
			callback, ok := found.(chan any)

			if ok {
				callback <- out
				close(callback)
			}
		}
	})

	live.client.OnConnected.Recurring(func(event *callback.Event[any]) {
		errnie.Info(fmt.Sprintf("websocket: connected to %s", live.client.URL))

		if auth {
			errnie.Error(live.authenticate())
			return
		}

		if err := live.restorePublicSubscriptions(); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"websocket: failed to restore public subscriptions",
				err,
			))
			live.setStatus(types.ERROR)
			return
		}

		live.setStatus(types.READY)
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

		live.setStatus(types.PENDING)

		bus := live.bus.Load()
		if bus != nil {
			bus.Notify(types.ChannelDisconnect)
		}
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

					live.setStatus(types.ERROR)
					return
				}
			}

			live.setStatus(types.READY)
		})
	}

	errnie.Info(fmt.Sprintf("websocket: connecting to %s", live.client.URL))
	live.setStatus(types.PENDING)

	if err := live.client.Connect(); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"websocket: failed to connect",
			err,
		))
	}

	return live
}

/*
captureFrame records one untouched websocket payload with its receive order and
canonical endpoint so the live feed is directly consumable by market replay.
*/
func (live *Live) captureFrame(endpoint string, payload []byte) error {
	if live.capture == nil {
		return nil
	}

	if endpoint == "" || len(payload) == 0 {
		return fmt.Errorf("websocket: capture endpoint and payload required")
	}

	// The SDK hands back a view into a buffer it reuses for the next frame,
	// so an asynchronously flushed recorder would write neighbouring frames
	// concatenated into it. The capture owns its own copy of the exact bytes.
	return live.capture.Capture(endpoint, bytes.Clone(payload), time.Now().UTC())
}

/*
CaptureSink receives one untouched transport payload with its endpoint and
arrival time. Implementations own persistence; the transport only reports.
*/
type CaptureSink interface {
	Capture(endpoint string, payload []byte, receivedAt time.Time) error
}

func (live *Live) Status() types.Status {
	status := live.status.Load()

	if status == nil {
		return types.UNKNOWN
	}

	return *status
}

func (live *Live) setStatus(status types.Status) {
	live.status.Store(&status)
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

func (live *Live) Client() *spot.WebSocket {
	return live.client
}

func (live *Live) SubInstrument(callback chan any) {
	errnie.Info("websocket: subscribing to instrument")

	live.Write(kraken.NewInstrumentSubscription(), Callback[any]{
		Channel: "instrument",
		Message: callback,
	})
}

func (live *Live) SubTicker(symbols []string) {
	live.rememberPublicSubscription("ticker", symbols)
	errnie.Error(live.client.SubTicker(symbols))
}

func (live *Live) SubBook(symbols []string) {
	live.rememberPublicSubscription("book", symbols)
	errnie.Error(live.client.SubBook(symbols, publicBookDepth))
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
	if len(symbols) == 0 {
		return
	}

	live.publicMu.Lock()
	live.public[channel] = append(live.public[channel], slices.Clone(symbols))
	live.publicMu.Unlock()
}

func (live *Live) restorePublicSubscriptions() error {
	live.publicMu.RLock()
	subscriptions := make(map[string][][]string, len(live.public))

	for channel, batches := range live.public {
		subscriptions[channel] = make([][]string, len(batches))

		for index, symbols := range batches {
			subscriptions[channel][index] = slices.Clone(symbols)
		}
	}

	live.publicMu.RUnlock()
	var err error

	for channel, batches := range subscriptions {
		for _, symbols := range batches {
			switch channel {
			case "ticker":
				err = errors.Join(err, live.client.SubTicker(symbols))
			case "book":
				err = errors.Join(err, live.client.SubBook(symbols, publicBookDepth))
			case "trade":
				err = errors.Join(err, live.client.SubTrades(symbols))
			case "ohlc":
				err = errors.Join(err, live.client.SubCandles(symbols))
			default:
				err = errors.Join(err, fmt.Errorf(
					"websocket: unsupported public subscription %s",
					channel,
				))
			}
		}
	}

	return err
}

func (live *Live) SubL3(symbols []string) {
	if live.level3 == nil {
		live.level3 = &sync.Map{}
	}

	for groups := range slices.Chunk(symbols, 200) {
		conn := NewWithClient(
			live.ctx,
			live.thesis.Load(),
			live.simulator,
			live.auth,
			Level3WebSocketURL,
			live.level3ClientFor(),
			live.capture,
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
		conn.SetFailureHandler(live.reportFailure)

		if observer := live.observer.Load(); observer != nil {
			conn.SetObserver(*observer)
		}

		conn.symbols = append([]string{}, groups...)
		live.subscribeLevel3Group(conn)
	}
}

/*
subscribeLevel3Group re-runs the exact paced level3 subscription batch the
startup path uses for one child connection. It is shared by boot and by a
checksum-divergence recovery so the two never diverge: both re-create the local
books and re-request the venue's level3 stream for the child's symbol group on
its already-connected socket.
*/
func (live *Live) subscribeLevel3Group(conn *Live) {
	if conn == nil || len(conn.symbols) == 0 {
		return
	}

	for group := range slices.Chunk(conn.symbols, 40) {
		if conn.book != nil {
			for _, symbol := range group {
				conn.book.Create(symbol, viper.GetInt("market.l3_depth"))
			}
		}

		conn.Client().SubL3(group, viper.GetInt("market.l3_depth"))
		time.Sleep(viper.GetDuration("market.subscribe.pace"))
	}
}

/*
AttachLevel3 installs an already-constructed level3 child connection into the
session's level3 map, so a fixture or injected transport can serve the book
manager without dialing the venue. It is the same registration SubL3 performs,
minus the venue client construction, and keeps the book lookup path unchanged.
*/
func (live *Live) AttachLevel3(groupKey string, conn *Live) {
	if live == nil || conn == nil || groupKey == "" {
		return
	}

	if live.level3 == nil {
		live.level3 = &sync.Map{}
	}

	live.level3.Store(groupKey, conn)
	conn.SetFailureHandler(live.reportFailure)

	if observer := live.observer.Load(); observer != nil {
		conn.SetObserver(*observer)
	}
}

/*
SetLevel3Client overrides the venue client SubL3 constructs for its child
connections. Fixtures set this to the level3 listener's own client so level3
subscriptions complete against the fixture instead of the real venue.
*/
func (live *Live) SetLevel3Client(factory func() *spot.WebSocket) {
	if live == nil {
		return
	}

	live.level3Client = factory
}

func (live *Live) level3ClientFor() *spot.WebSocket {
	if live != nil && live.level3Client != nil {
		return live.level3Client()
	}

	client := spot.NewWebSocket()
	client.URL = Level3WebSocketURL

	return client
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

	if len(response) > 0 {
		captureErr := live.captureFrame(TradeVolumeEndpoint, response)

		if captureErr != nil {
			return nil, captureErr
		}
	}

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
		live.callbacks.Store(callback.Channel, callback.Message)
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

	if live.level3 != nil {
		live.level3.Range(func(_, value any) bool {
			child, valid := value.(*Live)

			if valid && child != nil {
				child.Close()
			}

			return true
		})
	}

	if live.client.IsActive() {
		errnie.Error(live.client.Disconnect())
	}
}
