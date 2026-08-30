package websocket

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"

	"github.com/bytedance/sonic"
	gorillawebsocket "github.com/gorilla/websocket"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	sdkkraken "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
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
	ctx            context.Context
	cancel         context.CancelFunc
	status         *runtime.Status
	err            error
	client         *spot.WebSocket
	quote          string
	ingress        map[string]*runtime.Workload[*types.Envelope]
	simulator      *Simulator
	normalizer     *spot.Normalizer
	level3         *sync.Map
	symbols        []string
	publicMu       sync.RWMutex
	public         map[string][][]string
	auth           bool
	nonce          *AuthNonce
	nonceErr       error
	subscribers    *sync.Map
	callbacks      *sync.Map
	priceIncr      sync.Map
	paper          *Paper
	model          string
	capture        CaptureSink
	manifestSink   ManifestSink
	captureName    string
	failureMu      sync.RWMutex
	failure        func(error)
	observer       atomic.Pointer[func(string, time.Duration)]
	connectedCount atomic.Int32

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
New opens a spot websocket session and wires SDK callbacks in the constructor.
*/
func New(
	ctx context.Context,
	workloads map[string]*runtime.Workload[*types.Envelope],
	simulator *Simulator,
	auth bool,
	endpoint string,
	recorders ...CaptureSink,
) *Live {
	return NewWithClient(
		ctx, workloads, simulator, auth, endpoint, nil, recorders...,
	)
}

/*
NewWithClient opens a spot websocket session using an injected spot.WebSocket client instance.
A nil Thesis creates an explicit parsing-only session; SetThesis attaches event routing before
the connection becomes part of a running system.
*/
func NewWithClient(
	ctx context.Context,
	workloads map[string]*runtime.Workload[*types.Envelope],
	simulator *Simulator,
	auth bool,
	endpoint string,
	client *spot.WebSocket,
	recorders ...CaptureSink,
) *Live {
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

	if endpoint == system.Cfg.WebSocket.Endpoints.Level3 {
		captureName = "level3"
	}

	live := &Live{
		ctx:         ctx,
		cancel:      cancel,
		status:      runtime.NewStatus(),
		simulator:   simulator,
		client:      client,
		normalizer:  spot.NewNormalizer(),
		auth:        auth,
		subscribers: &sync.Map{},
		callbacks:   &sync.Map{},
		public:      make(map[string][][]string),
		paper:       NewPaper(ctx, NewSimulator(), workloads),
		ingress:     workloads,
		model:       viper.GetViper().GetString("trading.model"),
		quote:       viper.GetViper().GetString("market.quote_currency"),
		captureName: captureName,
	}

	if len(recorders) == 1 {
		live.capture = recorders[0]

		if manifestSink, ok := recorders[0].(ManifestSink); ok {
			live.manifestSink = manifestSink
		}
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

	if endpoint == system.Cfg.WebSocket.Endpoints.Level3 {
		live.level3 = &sync.Map{}
	}

	live.client.OnReceived.Recurring(func(event *callback.Event[*sdkkraken.WebSocketMessage]) {
		raw := event.Data.Bytes()
		channel := utils.GetString(raw, "channel")

		if channel == "" {
			if method := utils.GetString(raw, "method"); method != "" {
				channel = method
			}
		}

		// Every spot frame — public, private, or level3 — reaches the same
		// capture sink as the futures stream, so the events store sees the
		// whole system rather than futures alone. Capture mints the Hindsight
		// capture identity, persists the raw frame with it, and returns it so
		// every envelope parsed from this frame carries the exact same origin.
		var captureID hindsight.CaptureIdentity

		if live.capture != nil {
			captureID, _ = live.capture.Capture(channel, live.client.URL, bytes.Clone(raw), time.Now().UTC())
		}

		// An unsubscribe acknowledgement answers the instrument's paced
		// recovery of a checksum-diverged symbol; there is nothing to
		// dispatch for it.
		if channel == "unsubscribe" {
			return
		}

		handler, ok := entityMap[channel]

		if !ok {
			live.err = errnie.Error(errnie.Err(
				errnie.NotFound,
				"websocket: unhandled channel "+channel,
				nil,
			))

			live.status.Transition(runtime.ERROR)
			return
		}

		out := handler(raw)

		if channel == "subscribe" {
			errMessage := utils.GetString(raw, "error")

			if errMessage != "" {
				live.err = errnie.Error(errnie.Err(
					errnie.IO,
					fmt.Sprintf("[websocket] subscription rejected: %s", errMessage),
					nil,
				))

				live.status.Transition(runtime.ERROR)
				return
			}
		}

		// Dispatch one-shot callbacks (e.g. "instrument" snapshot)
		if cb, ok := live.callbacks.LoadAndDelete(channel); ok {
			if msgChan, ok := cb.(chan any); ok {
				msgChan <- out
			}
		}

		switch channel {
		case "pong":
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
		case "ticker", "trade", "level3":
			envelopes, manifests := IngestEnvelopes(channel, out, captureID)

			for index, envelope := range envelopes {
				live.ingress[channel].Push(envelope)

				if live.manifestSink != nil {
					_ = live.manifestSink.WriteManifest(manifests[index])
				}
			}
		}
	})

	live.client.OnConnected.Recurring(func(event *callback.Event[any]) {
		errnie.Info(fmt.Sprintf("websocket: connected to %s", live.client.URL))

		count := live.connectedCount.Add(1)

		if auth {
			errnie.Error(live.authenticate())
			return
		}

		if live.captureName == "level3" {
			if count > 1 && len(live.symbols) > 0 {
				live.subscribeLevel3Group(live)
			}
		}

		live.status.Transition(runtime.READY)
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

		live.status.Transition(runtime.WAITING)
	})

	if auth {
		live.client.OnAuthenticated.Recurring(func(event *callback.Event[string]) {
			errnie.Info(fmt.Sprintf("websocket: authenticated to %s", live.client.URL))

			if endpoint == system.Cfg.WebSocket.Endpoints.Private {
				err := live.subscribeAccount(event.Data)

				if err != nil {
					errnie.Error(errnie.Err(
						errnie.IO,
						"websocket: failed to subscribe to private account channels",
						err,
					))

					live.status.Transition(runtime.ERROR)
					return
				}
			}

			live.status.Transition(runtime.READY)
		})
	}

	errnie.Info(fmt.Sprintf("websocket: connecting to %s", live.client.URL))
	live.status.Transition(runtime.WAITING)

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
captureFrame records one untouched websocket payload with its origin kind,
receive order, and canonical endpoint so the live feed is directly consumable by
market replay.
*/
func (live *Live) captureFrame(kind, endpoint string, payload []byte) error {
	if live.capture == nil {
		return nil
	}

	if endpoint == "" || len(payload) == 0 {
		return fmt.Errorf("websocket: capture endpoint and payload required")
	}

	// The SDK hands back a view into a buffer it reuses for the next frame,
	// so an asynchronously flushed recorder would write neighbouring frames
	// concatenated into it. The capture owns its own copy of the exact bytes.
	_, err := live.capture.Capture(kind, endpoint, bytes.Clone(payload), time.Now().UTC())

	return err
}

/*
CaptureSink receives one untouched transport payload with its origin kind,
endpoint, and arrival time, and returns the CaptureIdentity it minted for that
frame. kind identifies the frame's channel/method/feed (e.g. "ticker", "trade",
"book", "level3", "pong"); endpoint names the stream it arrived on. The returned
identity is what the caller stamps onto every envelope parsed from the frame.
Implementations own persistence; the transport only reports.
*/
type CaptureSink interface {
	Capture(kind, endpoint string, payload []byte, receivedAt time.Time) (hindsight.CaptureIdentity, error)
}

/*
ManifestSink receives one EnvelopeManifest — how a raw frame entered Workspace —
keyed by its EnvelopeRef. A capture recorder that also implements this interface
gets the manifests for the envelopes it produced a capture identity for, so raw
capture and semantic ingress are persisted together and joinable by identity.
*/
type ManifestSink interface {
	WriteManifest(manifest hindsight.EnvelopeManifest) error
}

func (live *Live) Status() runtime.Stage {
	return live.status.Current()
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
	if live == nil {
		return nil
	}

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
	errnie.Error(live.client.SubTicker(symbols))
}

func (live *Live) SubTrades(symbols []string) {
	errnie.Error(live.client.SubTrades(symbols))
}

func (live *Live) SubL3(symbols []string) {
	if live.level3 == nil {
		live.level3 = &sync.Map{}
	}

	for groups := range slices.Chunk(symbols, 200) {
		conn := NewWithClient(
			live.ctx,
			live.ingress,
			live.simulator,
			live.auth,
			system.Cfg.WebSocket.Endpoints.Level3,
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
	if conn == nil || groupKey == "" {
		return
	}

	if live.level3 == nil {
		live.level3 = &sync.Map{}
	}

	live.level3.Store(groupKey, conn)
}

/*
SetLevel3Client overrides the venue client SubL3 constructs for its child
connections. Fixtures set this to the level3 listener's own client so level3
subscriptions complete against the fixture instead of the real venue.
*/
func (live *Live) SetLevel3Client(factory func() *spot.WebSocket) {
	live.level3Client = factory
}

func (live *Live) level3ClientFor() *spot.WebSocket {
	if live != nil && live.level3Client != nil {
		return live.level3Client()
	}

	client := spot.NewWebSocket()
	client.URL = system.Cfg.WebSocket.Endpoints.Level3

	return client
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
		captureErr := live.captureFrame("trade_volume", TradeVolumeEndpoint, response)

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
