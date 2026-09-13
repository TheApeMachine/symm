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
	"sync/atomic"
	"time"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
	"golang.design/x/lockfree/lf"

	"github.com/bytedance/sonic"
	gorillawebsocket "github.com/gorilla/websocket"
	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	sdkdecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	sdk "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
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
Live is one required spot websocket session. Operational disconnects replace
the venue connection, authenticate a fresh network session, and restore its
subscriptions; protocol and ingestion failures remain terminal.
*/
type Live struct {
	*runtime.System
	funding      FundingLedger
	queue        *lf.Queue[map[string]any]
	schema       map[string]data.Metric[float64]
	client       atomic.Pointer[spot.WebSocket]
	endpoint     string
	quote        string
	simulator    *Simulator
	normalizer   *spot.Normalizer
	book         *Book
	level3       *sync.Map
	symbols      []string
	auth         bool
	nonce        *AuthNonce
	nonceErr     error
	callbacks    *sync.Map
	paper        *Paper
	model        string
	pinger       *Pinger
	level3Client func() *spot.WebSocket
	pingReqID    atomic.Int64
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
	return NewWithClient(
		ctx, simulator, auth, endpoint, nil,
	)
}

/*
NewWithClient opens a spot websocket session using an injected spot.WebSocket client instance.
A nil Thesis creates an explicit parsing-only session; SetThesis attaches event routing before
the connection becomes part of a running system.
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

	// The SDK reconnect loop neither observes this session's context nor restores
	// subscriptions. Live owns that lifecycle and replaces the SDK client so a
	// rotated network address never inherits the previous venue session.
	client.Reconnect = nil
	client.OnDisconnected.Reset()

	name := "noname"

	switch endpoint {
	case system.Cfg.WebSocket.Endpoints.Public:
		name = "public"
	case system.Cfg.WebSocket.Endpoints.Private:
		name = "private"
	case system.Cfg.WebSocket.Endpoints.Level3:
		name = "level3"
	}

	live := &Live{
		System:     runtime.NewSystem(ctx, name),
		simulator:  simulator,
		endpoint:   endpoint,
		normalizer: spot.NewNormalizer(),
		auth:       auth,
		callbacks:  &sync.Map{},
		queue:      lf.NewQueue[map[string]any](),
		paper:      NewPaper(ctx, simulator),
		model:      system.Cfg.Market.Model,
		quote:      system.Cfg.Market.QuoteCurrency,
	}

	live.client.Store(client)

	live.pinger = NewPinger("websocket", func() error {
		if live.Status() != runtime.READY {
			return nil
		}

		ping, err := kraken.NewPing(live.pingReqID.Add(1)).MarshalJSON()

		if err != nil {
			return err
		}

		return live.client.Load().WriteMessage(gorillawebsocket.TextMessage, ping)
	})

	// A failed ping is the only evidence a half-open socket may produce. Treat it
	// exactly like a read-side disconnect and replace the venue session.
	live.pinger.OnFailed(func(err error) {
		go live.reconnect(err)
	})

	if err := live.normalizer.Use(live.client.Load().REST); err != nil {
		live.Error(errnie.Err(
			errnie.Validation,
			"websocket: failed to initialize normalizer",
			err,
		))

		return live
	}

	if auth {
		nonce, err := processAuthNonce()
		live.nonce = nonce
		live.nonceErr = err
		live.client.Load().REST.PublicKey = os.Getenv("KRAKEN_API_KEY")
		live.client.Load().REST.PrivateKey = os.Getenv("KRAKEN_API_SECRET")

		if live.nonceErr != nil || live.nonce == nil {
			live.Error(errnie.Err(
				errnie.Validation,
				"websocket: auth nonce unavailable",
				live.nonceErr,
			))

			return live
		}

		// Private and every Level3 batch authenticate with the same key; they
		// must share one monotonic nonce sequence or concurrent token fetches
		// collide (EAPI:Invalid nonce).
		live.client.Load().REST.Nonce = live.nonce.Next
	}

	if endpoint == system.Cfg.WebSocket.Endpoints.Level3 {
		live.level3 = &sync.Map{}
		live.book = NewBook(ctx, live.normalizer)
	}

	client.OnReceived.Recurring(func(event *callback.Event[*sdk.WebSocketMessage]) {
		if live.Status() != runtime.READY {
			return
		}

		raw := event.Data.Bytes()
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
			if message := utils.GetString(raw, "error"); message != "" {
				live.Error(errnie.Err(
					errnie.IO, "websocket: unsubscribe rejected: "+message, nil,
				))
			}

			return
		}

		switch channel {
		case "ticker", "trade", "executions":
			// One queue row per venue record: the callback splits the frame's
			// data array so Step converts exactly one measurement per dequeue.
			// The typed entity parse is skipped; the row map is the payload.
			frame, err := event.Data.Map()

			if err != nil {
				live.Error(errnie.Err(
					errnie.Validation,
					"websocket: failed to map "+channel+" frame",
					err,
				))

				return
			}

			rows, rowsOk := frame["data"].([]any)

			if !rowsOk {
				return
			}

			for _, entry := range rows {
				row, rowOk := entry.(map[string]any)

				if !rowOk {
					continue
				}

				row["channel"] = channel
				live.queue.Enqueue(row)
			}

			return
		}

		handler, ok := entityMap[channel]

		if !ok {
			live.Error(errnie.Err(
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
				live.Error(errnie.Err(
					errnie.IO,
					fmt.Sprintf("websocket: subscription rejected: %s", errMessage),
					nil,
				))

				return
			}
		}

		// Dispatch one-shot callbacks (e.g. "instrument" snapshot)
		if cb, ok := live.callbacks.LoadAndDelete(channel); ok {
			if msgChan, ok := cb.(chan any); ok {
				msgChan <- out
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

			if err := live.book.Update(event, level3); err != nil {
				errnie.Error(err)
			}

			return
		}

		switch channel {
		case "pong":

			if errMsg := utils.GetString(raw, "error"); errMsg != "" {
				live.Error(errnie.Err(
					errnie.IO,
					fmt.Sprintf("websocket: pong error: %s", errMsg),
					nil,
				))

				return
			}

			return
		}
	})

	errnie.Info(fmt.Sprintf("websocket: connecting to %s", live.client.Load().URL))
	live.Transition(runtime.WAITING)

	if err := live.client.Load().Connect(); err != nil {
		live.Error(errnie.Err(
			errnie.IO,
			"websocket: failed to connect",
			err,
		))
	} else if err := live.resume(); err != nil {
		live.Error(err)
	}

	return live
}

/*
Step implements the runtime.Node interface: one dequeued venue row becomes one
measurement. The queue's rows carry the venue's numbers as json.Number, so each
metric keeps the exact decimal the venue printed in Exact while Raw carries the
float64 the mathematics runs on.
*/
func (live *Live) Step(measurement *data.Measurement[float64]) *data.Measurement[float64] {
	row, ok := live.queue.Dequeue()

	if !ok {
		return measurement
	}

	if symbol, ok := row["symbol"].(string); ok {
		measurement.Label = live.normalizer.Name(symbol)
	}

	if side, ok := row["side"].(string); ok {
		if measurement.Provenance == nil {
			measurement.Provenance = make(map[string]string, 1)
		}

		measurement.Provenance["side"] = side
	}

	if stamped, ok := row["timestamp"].(string); ok {
		at, err := time.Parse(time.RFC3339Nano, stamped)

		if err != nil {
			measurement.Err = errnie.Err(errnie.Validation, "websocket: invalid row timestamp", err)
			return measurement
		}

		measurement.At = at
	}

	for key, value := range row {
		number, ok := value.(json.Number)

		if !ok {
			continue
		}

		exact, err := sdkdecimal.NewFromString(number.String())

		if err != nil {
			measurement.Err = errnie.Err(
				errnie.Validation,
				"websocket: invalid row number "+key,
				err,
			)

			return measurement
		}

		metric := measurement.Metrics[key]
		metric.Label = key
		metric.Raw = exact.Float64()
		metric.Exact = exact
		measurement.Metrics[key] = metric
	}

	return measurement
}

/*
Register implements the runtime.Node interface: it declares every numeric field
the venue's spot rows can produce, none valued.
*/
func (live *Live) Register() *data.Measurement[float64] {
	return data.NewMeasurement("websocket", map[string]data.Metric[float64]{})
}

func (live *Live) authenticate() (err error) {
	client := live.client.Load()
	errnie.Info(fmt.Sprintf("websocket[%s]: authenticating", client.URL))

	if live.nonceErr != nil {
		return live.Error(errnie.Err(
			errnie.Validation,
			"websocket: auth nonce unavailable",
			live.nonceErr,
		))
	}

	if err = client.Authenticate(); err != nil && !strings.Contains(
		err.Error(), "Invalid nonce",
	) {
		return live.Error(errnie.Err(
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

	return client.Authenticate()
}

/*
resume establishes the authenticated identity and subscriptions belonging to a
fresh venue connection. A reconnect fetches a new token instead of carrying the
previous network session across a possible client-IP change.
*/
func (live *Live) resume() error {
	if live.auth {
		if err := live.authenticate(); err != nil {
			return errnie.Err(
				errnie.Unauthorized,
				"websocket: authentication failed",
				err,
			)
		}

		errnie.Info(fmt.Sprintf(
			"websocket: authenticated to %s", live.client.Load().URL,
		))
	}

	if live.endpoint == system.Cfg.WebSocket.Endpoints.Private {
		if err := live.subscribeAccount(live.client.Load().Token); err != nil {
			return errnie.Err(
				errnie.IO,
				"websocket: failed to restore private account subscriptions",
				err,
			)
		}

		return nil
	}

	live.Transition(runtime.READY)
	return nil
}

/*
reconnect replaces the SDK client and retries until a complete venue session is
ready or the owning context is canceled. Every attempt uses the SDK's configured
retry cadence and, for authenticated sockets, obtains a new websocket token.
*/
func (live *Live) reconnect(err error) {
	live.pinger.Stop()
	live.Transition(runtime.WAITING)
	retryWait := live.client.Load().ReconnectWait
	errnie.Error(errnie.Err(
		errnie.IO,
		fmt.Sprintf("websocket %s disconnected; reconnecting with a fresh session", live.endpoint),
		err,
	))

	for live.Context().Err() == nil {
		client := live.client.Load()
		replacement := spot.NewWebSocket()
		replacement.REST = client.REST
		replacement.URL = client.URL
		replacement.Reconnect = nil
		replacement.ReconnectWait = client.ReconnectWait
		replacement.Insecure = client.Insecure
		replacement.OnAuthenticated = client.OnAuthenticated
		replacement.OnSent = client.OnSent
		live.client.Store(replacement)

		// Ask a still-readable retired peer to close. SDK Disconnect cannot be
		// used after a failed write: v2.0.0 leaves a callback sending to a closed
		// channel on that error path. Session identity already rejects its data.
		if err := client.WriteMessage(gorillawebsocket.CloseMessage,
			gorillawebsocket.FormatCloseMessage(gorillawebsocket.CloseNormalClosure, "")); err != nil {
			errnie.Warn("websocket: retired session close: " + err.Error())
		}

		err = replacement.Connect()
		established := err == nil

		if established {
			err = live.resume()
		}

		if err == nil {
			return
		}

		live.pinger.Stop()
		live.Transition(runtime.WAITING)

		live.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("websocket %s fresh-session reconnect failed", live.endpoint),
			err,
		))

		retry := time.NewTimer(retryWait)

		select {
		case <-retry.C:
		case <-live.Context().Done():
			retry.Stop()
			return
		}
	}
}

/*
subscribeAccount activates Kraken's private wallet and execution streams after
authentication. Kraken closes an authenticated socket that does not submit a
private subscription within its token deadline.

The execution subscription fans out into the executions ingress workload, which
is wired after the desk exists — later in boot than the private session's first
authenticate. Subscribing before that workload is ready would deliver execution
frames into a nil workload. The subscription therefore waits on the executions
workload's readiness before it writes, within the token deadline.
*/
func (live *Live) subscribeAccount(token string) error {
	// The balance subscription is submitted immediately: it satisfies Kraken's
	// authenticated-socket deadline (which closes a socket that submits no
	// private subscription) and its frames flow through callbacks, never the
	// executions ingress.
	if err := live.Write(kraken.NewBalanceSubscription(token)); err != nil {
		return err
	}

	return nil
}

/*
subscribeExecutionsWhenReady submits the private execution subscription once the
executions ingress workload is ready. It owns the gate so the initial
authenticate returns immediately (and the balance subscription already satisfied
the token deadline), while execution frames cannot arrive before their consumer.
*/
func (live *Live) SubExecutions(token string) {
	if err := live.client.Load().SubExecutions(); err != nil {
		live.Error(errnie.Err(
			errnie.IO,
			"websocket: failed to subscribe to executions",
			err,
		))
	}
}

func (live *Live) SubInstrument(callback chan any) {
	errnie.Info("websocket: subscribing to instrument")

	if err := live.Write(kraken.NewInstrumentSubscription(), Callback[any]{
		Channel: "instrument",
		Message: callback,
	}); err != nil {
		live.Error(errnie.Err(
			errnie.IO,
			"websocket: failed to subscribe to instruments",
			err,
		))
	}
}

func (live *Live) SubTicker(symbols []string) {
	if live.Status() != runtime.BUSY && live.Status() != runtime.READY {
		live.Error(errnie.Err(
			errnie.NotAcceptable,
			"websocket: ticker subscription requires a connected session",
			nil,
		))

		return
	}

	if err := live.client.Load().SubTicker(symbols); err != nil {
		live.Error(errnie.Err(
			errnie.IO,
			"websocket: failed to subscribe to ticker",
			err,
		))

		return
	}
}

func (live *Live) SubTrades(symbols []string) {
	if live.Status() != runtime.BUSY && live.Status() != runtime.READY {
		live.Error(errnie.Err(
			errnie.NotAcceptable,
			"websocket: trade subscription requires a connected session",
			nil,
		))

		return
	}

	if err := live.client.Load().SubTrades(symbols); err != nil {
		live.Error(errnie.Err(
			errnie.IO,
			"websocket: failed to subscribe to trades",
			err,
		))

		return
	}
}

func (live *Live) SubL3(symbols []string) {
	if live.Status() != runtime.BUSY && live.Status() != runtime.READY {
		live.Error(errnie.Err(
			errnie.NotAcceptable,
			"websocket: level3 subscription requires a connected session",
			nil,
		))

		return
	}

	if live.level3 == nil {
		live.level3 = &sync.Map{}
	}

	for groups := range slices.Chunk(symbols, 200) {
		groupKey := strings.Join(groups, "|")

		existing, loaded := live.level3.Load(groupKey)

		if loaded {
			conn, valid := existing.(*Live)

			if valid && conn != nil && conn.Error() == nil {
				if err := live.subscribeLevel3Group(conn); err != nil {
					live.Error(err)
				}

				continue
			}
		}

		conn := NewWithClient(
			live.Context(),
			live.simulator,
			live.auth,
			system.Cfg.WebSocket.Endpoints.Level3,
			live.level3ClientFor(),
		)

		if conn.Error() != nil {
			return
		}

		conn.symbols = append([]string{}, groups...)
		live.AttachLevel3(groupKey, conn)

		if err := live.subscribeLevel3Group(conn); err != nil {
			live.Error(err)
			return
		}
	}
}

/*
subscribeLevel3Group re-runs the exact paced level3 subscription batch the
startup path uses for one child connection. Boot and fresh-session reconnects
therefore apply the same venue pacing and request the same symbol group.
*/
func (live *Live) subscribeLevel3Group(conn *Live) error {
	if conn == nil || len(conn.symbols) == 0 {
		return nil
	}

	if conn.Status() != runtime.BUSY && conn.Status() != runtime.READY {
		return errnie.Err(
			errnie.NotAcceptable,
			"websocket: level3 child subscription requires a connected session",
			nil,
		)
	}

	for group := range slices.Chunk(conn.symbols, 40) {
		conn.book.Expect(group)
		if err := conn.client.Load().SubPrivate("level3", map[string]any{
			"params": map[string]any{"symbol": group, "depth": viper.GetInt("market.l3_depth"), "snapshot": true},
		}); err != nil {
			err = errnie.Err(
				errnie.IO,
				"websocket: failed to subscribe to level3",
				err,
			)

			return err
		}

		if err := conn.book.Wait(); err != nil {
			return err
		}

		time.Sleep(viper.GetDuration("market.subscribe.pace"))
	}

	return nil
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

		if !ok || conn.book == nil || conn.Status() != runtime.READY {
			return true
		}

		conn.book.Book(symbol, func(managed *book.Book) {
			found = true
			read(managed)
		})
		return !found
	})

	if !found {
		read(nil)
	}
}

func (live *Live) Balance() (*kraken.Balance, error) {
	if live.model == "real" {
		response, err := live.client.Load().REST.Balances()

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.IO,
				"balance: failed to fetch",
				err,
			))
		}

		return kraken.NewBalanceFromMap(response.Result), nil
	}

	return live.paper.Balances()
}

func (live *Live) TradesHistory() (spot.TradesHistoryResult, error) {
	if live.model == "real" {
		result := spot.TradesHistoryResult{Trades: map[string]spot.Trade{}}
		offset := 0

		for {
			response, err := live.client.Load().REST.TradesHistory(&spot.TradesHistoryRequest{
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
	if live.model == "real" {
		response, err := live.client.Load().REST.OpenOrders(&spot.OpenOrdersRequest{Trades: true})

		if err != nil {
			return spot.OpenOrdersResult{}, errnie.Error(err)
		}

		return response.Result, nil
	}

	return live.paper.OpenOrders()
}

func (live *Live) CancelOrder(
	request *spot.CancelOrderRequest,
) (spot.CancelResult, error) {
	if live.model == "real" {
		response, err := live.client.Load().REST.CancelOrder(request)

		if err != nil {
			return spot.CancelResult{}, errnie.Error(err)
		}

		return response.Result, nil
	}

	return live.paper.CancelOrder(request)
}

func (live *Live) TradeBalance() (*kraken.TradeBalanceResult, error) {
	if live.model == "real" {
		before, _, err := live.funding.Observe(
			live.Post,
			live.normalizer.Name,
			live.quote,
			time.Now().UTC(),
		)

		if err != nil {
			errnie.Error(err)
		}

		response, err := live.Post(
			system.Cfg.WebSocket.Endpoints.TradeBalance,
			kraken.NewTradeBalanceRequest(live.quote),
		)

		if err != nil {
			return nil, errnie.Error(err)
		}

		result := kraken.NewTradeBalance(response)
		complete := result.EquivalentBalance != nil
		result.ValuationComplete = &complete
		balances, err := live.Post("/0/private/BalanceEx", json.RawMessage(`{}`))

		if err != nil {
			return result, errnie.Error(err)
		}

		extended, err := kraken.NewExtendedBalance(balances)

		if err != nil {
			return result, errnie.Error(err)
		}

		result.AvailableCash, err = extended.Available(live.quote, live.normalizer.Name)

		if err != nil {
			return result, errnie.Error(err)
		}

		result.NetFunding, result.FundingReason, err = live.funding.Observe(
			live.Post, live.normalizer.Name, live.quote, time.Now().UTC(),
		)

		if err != nil {
			errnie.Error(err)
		}

		if before == nil || (result.NetFunding != nil && before.Cmp(result.NetFunding) != 0) {
			result.NetFunding = nil
			result.FundingReason = "funding changed or was unavailable during valuation"
		}
		return result, nil
	}

	return live.paper.TradeBalance()
}

func (live *Live) TradeVolume(symbols []string) (*kraken.TradeVolumeResult, error) {
	if live.model != "real" {
		return live.paper.TradeVolume(symbols)
	}

	response, err := live.Post(
		system.Cfg.WebSocket.Endpoints.TradeVolume,
		kraken.NewTradeVolumeRequest(symbols),
	)

	return kraken.NewTradeVolume(response), errnie.Error(err)
}

func (live *Live) AddOrder(order *spot.AddOrderRequest) (spot.AddOrderResult, error) {
	// Only a real model reaches the venue. The test read the other way round,
	// which sent paper orders to Kraken over REST and routed real ones into
	// the simulator.
	if live.model == "real" {
		response, err := live.client.Load().REST.AddOrder(order)

		if err != nil {
			return spot.AddOrderResult{}, errnie.Error(errnie.Err(
				errnie.IO,
				"[live] add order failed to submit",
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
		return live.Error(errnie.Err(
			errnie.Validation,
			"[live] write marshal failed",
			err,
		))
	}

	started := time.Now()

	err = live.client.Load().WriteMessage(
		gorillawebsocket.TextMessage, raw,
	)

	if err != nil {
		return live.Error(errnie.Err(
			errnie.IO,
			"[live] write failed",
			err,
		))
	}

	if live.simulator != nil {
		live.simulator.Record(WEBSOCKET, time.Since(started))
	}

	return nil
}

func (live *Live) do(options spot.RequestOptions) ([]byte, error) {
	started := time.Now()
	request, err := live.client.Load().REST.NewRequest(options)

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
