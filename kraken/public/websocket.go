package public

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/types"
)

/*
WebSocket is the Kraken public websocket client.
*/
type WebSocket struct {
	ctx             context.Context
	cancel          context.CancelFunc
	err             error
	tree            *dmt.Tree
	pool            *qpool.Q[any]
	broadcasts      *sync.Map
	subscribers     *sync.Map
	conn            *websocket.Conn
	isConnected     atomic.Bool
	connectMaxDelay int
	connectDelay    int
}

var dialWebSocket = websocket.DefaultDialer.Dial

/*
NewWebSocket creates a new Kraken public websocket client.
*/
func NewWebSocket(
	ctx context.Context, pool *qpool.Q[any], tree *dmt.Tree,
) *WebSocket {
	ctx, cancel := context.WithCancel(ctx)

	socket := &WebSocket{
		ctx:             ctx,
		cancel:          cancel,
		tree:            tree,
		pool:            pool,
		broadcasts:      &sync.Map{},
		subscribers:     &sync.Map{},
		isConnected:     atomic.Bool{},
		connectMaxDelay: viper.GetInt("system.network.connection.max_delay"),
	}

	for _, channel := range []string{"ticker"} {
		socket.broadcasts.Store(channel, pool.CreateBroadcastGroup("ticker"))
	}

	for _, channel := range []string{"kraken:public"} {
		socket.subscribers.Store(
			channel, pool.Subscribe(channel, socket.onMessage),
		)
	}

	errnie.Info("kraken/public: websocket client ready")
	return socket
}

/*
onMessage will be called by the qpool.BroadcastGroup for every consumer
that has subscribed with a callback function.
*/
func (ws *WebSocket) onMessage(artifact *datura.Artifact) error {
	destination := errnie.Does(func() (string, error) {
		return artifact.Destination()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken/public: failed to get destination",
			err,
		))
	}).Value()

	switch destination {
	case "kraken:public":
		if ws.conn == nil || !ws.isConnected.Load() {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken/public: websocket not connected",
				errors.New("not connected"),
			))
		}

		payload := errnie.Does(func() ([]byte, error) {
			return artifact.DecryptPayload(), nil
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken/public: failed to get payload",
				err,
			))
		}).Value()

		return errnie.Error(ws.send(payload))
	default:
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken/public: ignored destination",
			errors.New(destination),
		))
	}
}

/*
send writes one frame to the connection.
*/
func (ws *WebSocket) send(payload []byte) error {
	if ws == nil || ws.conn == nil || !ws.isConnected.Load() {
		return errnie.Err(errnie.Validation, "kraken/public: websocket not connected", nil)
	}

	return ws.conn.WriteMessage(websocket.TextMessage, payload)
}

/*
Run reads Kraken public websocket frames and routes them into broadcast groups.
*/
func (ws *WebSocket) Run(endpoint EndpointType) {
	errnie.Info("kraken/public: websocket client running")

	for {
		select {
		case <-ws.ctx.Done():
			return
		default:
		}

		if !ws.isConnected.Load() || ws.conn == nil {
			if err := ws.Connect(endpoint, 0); err != nil {
				if ws.ctx.Err() != nil {
					return
				}
				ws.err = errnie.Err(errnie.IO, "kraken/public: websocket connect failed", err)
				errnie.Error(ws.err)
				continue
			}
		}

		if err := ws.readOnce(); err != nil {
			ws.err = err
			ws.disconnect()
			errnie.Error(err)
		}
	}
}

func (ws *WebSocket) readOnce() error {
	_, message, err := ws.conn.ReadMessage()
	if err != nil {
		return errnie.Err(
			errnie.IO,
			"kraken/public: failed to read message",
			err,
		)
	}

	var wire types.SocketMessage
	if err := sonic.Unmarshal(message, &wire); err != nil {
		return errnie.Err(errnie.IO, "kraken/public: decode message", err)
	}

	if wire.Channel == "ticker" {
		bg, ok := ws.broadcasts.Load("ticker")
		if !ok {
			return errnie.Err(errnie.Validation, "kraken/public: ticker broadcast missing", nil)
		}

		if err := bg.(*qpool.BroadcastGroup).Send(datura.Acquire(
			"kraken:public", datura.APPJSON,
		).WithDestination(
			"desk",
		).WithRole(
			"ticker",
		).WithScope(
			"update",
		).WithPayload(
			message,
		)); err != nil {
			return errnie.Err(errnie.Validation, "kraken/public: send ticker to desk", err)
		}
	}

	// level3 (Level3Channel) is persisted exactly like book/trade: role=level3,
	// scope=symbol, same role/timestamp and role/scope/timestamp indexes.
	// Per-order add/delete/fill events (order_id/limit_price/order_qty) land
	// in the tree so signals can later seek level3/{symbol}/… for cancel-vs-fill,
	// order age, and spoof shape.
	if !slices.Contains(
		[]string{"ohlc", "instrument", "ticker", "book", "trade", Level3Channel},
		wire.Channel,
	) {
		return nil
	}

	// Instrument frames are the venue catalog, not time-series: each pair is
	// stored once per symbol (latest wins) and drives discovery. Everything
	// else is appended as history keyed role-first with a timestamp cursor.
	if wire.Channel == "instrument" {
		if err := ws.discover(wire.Data, wire.Type); err != nil {
			return errnie.Err(errnie.Validation, "kraken/public: discover instrument frame", err)
		}
		return nil
	}

	return ws.persistMarketFrame(wire)
}

type symbolFrame struct {
	Channel string            `json:"channel"`
	Type    string            `json:"type"`
	Data    []json.RawMessage `json:"data"`
	TimeIn  time.Time         `json:"time_in,omitempty"`
	TimeOut time.Time         `json:"time_out,omitempty"`
}

func (ws *WebSocket) persistMarketFrame(wire types.SocketMessage) error {
	rows, order, rowsErr := rowsBySymbol(wire.Data)
	if rowsErr != nil {
		return errnie.Err(errnie.Validation, "kraken/public: parse symbol rows", rowsErr)
	}

	for _, symbol := range order {
		if !symbolMatchesQuoteCurrency(symbol) {
			continue
		}

		payload, err := sonic.Marshal(symbolFrame{
			Channel: wire.Channel,
			Type:    wire.Type,
			Data:    rows[symbol],
			TimeIn:  wire.TimeIn,
			TimeOut: wire.TimeOut,
		})

		if err != nil {
			return errnie.Err(
				errnie.Validation,
				"kraken/public: failed to marshal symbol frame",
				err,
			).With("channel", wire.Channel, "symbol", symbol)
		}

		artifact := datura.Acquire(
			"websocket", datura.APPJSON,
		).WithRole(
			wire.Channel,
		).WithScope(
			symbol,
		).WithPayload(
			payload,
		)

		if err := ws.insertMarketArtifact(artifact); err != nil {
			return err
		}
	}

	return nil
}

func rowsBySymbol(data json.RawMessage) (map[string][]json.RawMessage, []string, error) {
	var rows []json.RawMessage
	if err := sonic.Unmarshal(data, &rows); err != nil {
		return nil, nil, err
	}

	grouped := make(map[string][]json.RawMessage, len(rows))
	order := make([]string, 0, len(rows))

	for _, raw := range rows {
		var row struct {
			Symbol string `json:"symbol"`
		}

		if err := sonic.Unmarshal(raw, &row); err != nil {
			return nil, nil, err
		}
		if row.Symbol == "" {
			return nil, nil, errnie.Err(errnie.Validation, "kraken/public: market row missing symbol", nil)
		}

		if _, ok := grouped[row.Symbol]; !ok {
			order = append(order, row.Symbol)
		}

		grouped[row.Symbol] = append(grouped[row.Symbol], raw)
	}

	return grouped, order, nil
}

func (ws *WebSocket) insertMarketArtifact(artifact *datura.Artifact) error {
	if ws == nil || ws.tree == nil || artifact == nil {
		return errnie.Err(errnie.Validation, "kraken/public: nil market artifact insert state", nil)
	}

	packed := artifact.Pack()
	if len(packed) == 0 {
		return errnie.Err(errnie.Validation, "kraken/public: market artifact packed empty", nil)
	}

	ws.tree.Insert(artifact.Prefix("role", "timestamp"), packed)
	ws.tree.Insert(artifact.Prefix("role", "scope", "timestamp"), packed)

	return nil
}

/*
discover parses an instrument frame, persists each online pair's full metadata
(tick_size, qty_min, precision, …) to the tree under instrument/{symbol}, and
subscribes ticker/book/trade for pairs not already in the tree. The instrument
feed is the symbol source of truth — no default list, no symbol cap. The tree is
the only store, so dedup is "is this pair already recorded?", not a side map.
*/
func (ws *WebSocket) discover(data json.RawMessage, frameType string) error {
	fresh, err := ws.recordPairs(data, frameType)

	if err != nil {
		return errnie.Err(errnie.Validation, "kraken/public: failed to parse instrument frame", err)
	}

	if len(fresh) == 0 {
		return nil
	}

	errnie.Info(fmt.Sprintf("kraken/public: subscribing %d online pairs", len(fresh)))
	return ws.subscribeData(fresh)
}

/*
instrumentKey is the stable per-symbol catalog key. It carries no timestamp so
the latest definition overwrites the prior one and consumers can Get it directly.
*/
func instrumentKey(symbol string) []byte {
	return []byte("instrument/" + symbol + "/")
}

/*
recordPairs writes every online pair's metadata to the tree and returns the
symbols that were newly recorded (not previously in the tree) so they can be
subscribed. Pure of socket I/O so it is testable.
*/
func (ws *WebSocket) recordPairs(data json.RawMessage, frameType string) ([]string, error) {
	var frame struct {
		Pairs []json.RawMessage `json:"pairs"`
	}

	if err := sonic.Unmarshal(data, &frame); err != nil {
		return nil, err
	}

	// A snapshot is the full catalog and arrives once per connection, so every
	// online pair is (re)subscribed — this is what restores subscriptions after
	// a reconnect. An update carries only changed pairs, so only ones not yet in
	// the tree are new and need subscribing.
	snapshot := frameType == "snapshot"
	fresh := make([]string, 0, len(frame.Pairs))

	for _, raw := range frame.Pairs {
		var meta struct {
			Symbol string `json:"symbol"`
			Status string `json:"status"`
		}

		if err := sonic.Unmarshal(raw, &meta); err != nil {
			return nil, err
		}

		if meta.Symbol == "" {
			return nil, errnie.Err(errnie.Validation, "kraken/public: instrument row missing symbol", nil)
		}

		if meta.Status != "online" || !symbolMatchesQuoteCurrency(meta.Symbol) {
			continue
		}

		key := instrumentKey(meta.Symbol)
		_, known := ws.tree.Get(key)

		// Persist (or refresh) the full per-pair metadata so consumers can seek
		// instrument/{symbol} for tick_size, qty_min, precision, etc.
		ws.tree.InsertArtifact(key, datura.Acquire(
			"websocket", datura.APPJSON,
		).WithRole(
			"instrument",
		).WithScope(
			meta.Symbol,
		).WithPayload(
			raw,
		))

		if snapshot || !known {
			fresh = append(fresh, meta.Symbol)
		}
	}

	return fresh, nil
}

func symbolMatchesQuoteCurrency(symbol string) bool {
	quote := strings.ToUpper(strings.TrimSpace(viper.GetString("market.quote_currency")))

	if quote == "" {
		return !symbolHasExcludedBase(symbol)
	}

	base, symbolQuote, ok := strings.Cut(symbol, "/")

	return ok && strings.ToUpper(symbolQuote) == quote && !excludedBase(base)
}

func symbolHasExcludedBase(symbol string) bool {
	base, _, ok := strings.Cut(symbol, "/")
	return ok && excludedBase(base)
}

func excludedBase(base string) bool {
	switch strings.ToUpper(strings.TrimSpace(base)) {
	case "AUD", "BTC", "CAD", "CHF", "DAI", "EUR", "EURS", "EURT", "GBP",
		"JPY", "PYUSD", "TUSD", "USD", "USDC", "USDE", "USDP", "USDS",
		"USDT", "UST", "USTC", "XBT":
		return true
	default:
		return false
	}
}

/*
subscribeData subscribes ticker/book/trade for the discovered symbols. Kraken
closes the connection on an oversized subscribe frame and the universe is
hundreds of pairs, so symbols are sent in chunks of market.subscribe_batch.
Writes happen inline on the read-loop goroutine (the sole caller of discover),
keeping a single writer with no locking; each chunk frame is small and fast, so
reads resume between them.
*/
func (ws *WebSocket) subscribeData(symbols []string) error {
	depth := viper.GetInt("market.book.depth")
	if depth <= 0 {
		depth = viper.GetInt("market.book_depth_levels")
	}
	if depth <= 0 {
		depth = 10
	}

	batch := viper.GetInt("market.subscribe_batch")
	if batch <= 0 {
		batch = 100
	}

	for start := 0; start < len(symbols); start += batch {
		end := min(start+batch, len(symbols))
		symbolJSON, marshalErr := sonic.Marshal(symbols[start:end])
		if marshalErr != nil {
			return errnie.Err(errnie.Validation, "kraken/public: marshal subscribe symbols", marshalErr)
		}

		if err := ws.send([]byte(fmt.Sprintf(
			`{"method": "subscribe","params": {"channel": "ticker", "symbol": %s}}`,
			string(symbolJSON),
		))); err != nil {
			return errnie.Err(errnie.IO, "kraken/public: subscribe ticker", err)
		}

		if err := ws.send([]byte(fmt.Sprintf(
			`{"method": "subscribe","params": {"channel": "book", "depth": %d, "symbol": %s}}`,
			depth,
			string(symbolJSON),
		))); err != nil {
			return errnie.Err(errnie.IO, "kraken/public: subscribe book", err)
		}

		if err := ws.send([]byte(fmt.Sprintf(
			`{"method": "subscribe","params": {"channel": "trade", "symbol": %s}}`,
			string(symbolJSON),
		))); err != nil {
			return errnie.Err(errnie.IO, "kraken/public: subscribe trade", err)
		}

		// L3 is authenticated and lives on WebSocketL3URL, not this public socket.
		// The read loop above will persist level3 frames when an authenticated L3
		// connection supplies them; do not send a known-invalid public subscribe.

		if pace := viper.GetDuration("market.subscribe_pace"); pace > 0 && end < len(symbols) {
			time.Sleep(pace)
		}
	}

	return nil
}

/*
Error returns the error of the Kraken public websocket.
*/
func (ws *WebSocket) Error() error {
	return ws.err
}

/*
Close closes the Kraken public websocket.
*/
func (ws *WebSocket) Close() (err error) {
	if ws.conn != nil {
		err = errnie.Guard(
			errnie.IO,
			"kraken/public: failed to close connection",
			errnie.Error(ws.conn.Close()),
		)
	}

	ws.isConnected.Store(false)
	ws.cancel()
	return err
}

/*
Connect connects to the Kraken public websocket using the configured bounded
reconnect delay. The signature stays stable for existing callers; the attempt
argument is ignored by the iterative implementation.
*/
func (ws *WebSocket) Connect(endpoint EndpointType, _ int) error {
	if ws.isConnected.Load() && ws.conn != nil {
		return nil
	}

	delay := ws.reconnectInitial()
	maxDelay := ws.reconnectMax()

	for {
		if err := ws.ctx.Err(); err != nil {
			return err
		}

		errnie.Info("kraken/public: websocket client dialing")

		conn, response, err := dialWebSocket(string(endpoint), http.Header{})
		ws.err = err

		if err == nil && response != nil && response.StatusCode == http.StatusSwitchingProtocols {
			ws.conn = conn
			ws.isConnected.Store(true)
			ws.connectDelay = 1
			errnie.Info("kraken/public: websocket connected")

			if err := ws.subscribeAll(); err != nil {
				ws.disconnect()
				ws.err = err
			} else {
				return nil
			}
		} else {
			if conn != nil {
				_ = conn.Close()
			}
			status := 0
			if response != nil {
				status = response.StatusCode
			}
			ws.err = errnie.Err(
				errnie.IO,
				"kraken/public: websocket dial failed",
				err,
			).With("status", status)
		}

		if !sleepContext(ws.ctx, delay) {
			return ws.ctx.Err()
		}
		delay = nextReconnectDelay(delay, maxDelay, ws.reconnectMultiplier())
	}
}

func (ws *WebSocket) disconnect() {
	ws.isConnected.Store(false)
	if ws.conn != nil {
		_ = ws.conn.Close()
		ws.conn = nil
	}
}

func (ws *WebSocket) reconnectInitial() time.Duration {
	delay := viper.GetDuration("market.ws_reconnect_initial")
	if delay <= 0 {
		delay = time.Second
	}
	return delay
}

func (ws *WebSocket) reconnectMax() time.Duration {
	delay := viper.GetDuration("market.ws_reconnect_max")
	if delay <= 0 {
		if ws.connectMaxDelay > 0 {
			delay = time.Duration(ws.connectMaxDelay) * time.Second
		} else {
			delay = 30 * time.Second
		}
	}
	return delay
}

func (ws *WebSocket) reconnectMultiplier() float64 {
	multiplier := viper.GetFloat64("market.ws_reconnect_multiplier")
	if multiplier < 1 {
		multiplier = 2
	}
	return multiplier
}

func nextReconnectDelay(current, maxDelay time.Duration, multiplier float64) time.Duration {
	if maxDelay <= 0 {
		return current
	}
	next := time.Duration(float64(current) * multiplier)
	if next <= current {
		next = current
	}
	if next > maxDelay {
		return maxDelay
	}
	return next
}

func sleepContext(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

/*
subscribeAll subscribes only to the instrument channel. The instrument snapshot
drives ticker/book/trade subscriptions per discovered pair via discover.
*/
func (ws *WebSocket) subscribeAll() error {
	// Request the instrument snapshot; recordPairs re-subscribes every online
	// pair from it, which restores data subscriptions after a reconnect.
	if err := ws.send([]byte(
		`{"method": "subscribe","params": {"channel": "instrument", "snapshot": true}}`,
	)); err != nil {
		return errnie.Err(errnie.IO, "kraken/public: subscribe instrument", err)
	}

	return nil
}
