package public

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"slices"
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
		subscribers:     &sync.Map{},
		isConnected:     atomic.Bool{},
		connectMaxDelay: viper.GetInt("system.network.connection.max_delay"),
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
			errnie.Error(ws.Connect(endpoint, 0))
			continue
		}

		_, message, err := ws.conn.ReadMessage()

		if err != nil {
			ws.err = errnie.Error(errnie.Err(
				errnie.IO,
				"kraken/public: failed to read message",
				err,
			))
			ws.isConnected.Store(false)
			continue
		}

		var wire types.SocketMessage
		errnie.Error(sonic.Unmarshal(message, &wire))

		// level3 (Level3Channel) is persisted exactly like book/trade: role=level3,
		// scope=symbol, same role/timestamp and role/scope/timestamp indexes.
		// Per-order add/delete/fill events (order_id/limit_price/order_qty) land
		// in the tree so signals can later seek level3/{symbol}/… for cancel-vs-fill,
		// order age, and spoof shape.
		if !slices.Contains(
			[]string{"ohlc", "instrument", "ticker", "book", "trade", Level3Channel},
			wire.Channel,
		) {
			continue
		}

		// Instrument frames are the venue catalog, not time-series: each pair is
		// stored once per symbol (latest wins) and drives discovery. Everything
		// else is appended as history keyed role-first with a timestamp cursor.
		if wire.Channel == "instrument" {
			ws.discover(wire.Data, wire.Type)
			continue
		}

		ws.persistMarketFrame(wire)
	}
}

type symbolFrame struct {
	Channel string            `json:"channel"`
	Type    string            `json:"type"`
	Data    []json.RawMessage `json:"data"`
	TimeIn  time.Time         `json:"time_in,omitempty"`
	TimeOut time.Time         `json:"time_out,omitempty"`
}

func (ws *WebSocket) persistMarketFrame(wire types.SocketMessage) {
	rows, order := rowsBySymbol(wire.Data)

	for _, symbol := range order {
		payload, err := sonic.Marshal(symbolFrame{
			Channel: wire.Channel,
			Type:    wire.Type,
			Data:    rows[symbol],
			TimeIn:  wire.TimeIn,
			TimeOut: wire.TimeOut,
		})

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken/public: failed to marshal symbol frame",
				err,
			).With("channel", wire.Channel, "symbol", symbol))
			continue
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

		ws.insertMarketArtifact(artifact)
	}
}

func rowsBySymbol(data json.RawMessage) (map[string][]json.RawMessage, []string) {
	var rows []json.RawMessage
	if err := sonic.Unmarshal(data, &rows); err != nil {
		return nil, nil
	}

	grouped := make(map[string][]json.RawMessage, len(rows))
	order := make([]string, 0, len(rows))

	for _, raw := range rows {
		var row struct {
			Symbol string `json:"symbol"`
		}

		if err := sonic.Unmarshal(raw, &row); err != nil || row.Symbol == "" {
			continue
		}

		if _, ok := grouped[row.Symbol]; !ok {
			order = append(order, row.Symbol)
		}

		grouped[row.Symbol] = append(grouped[row.Symbol], raw)
	}

	return grouped, order
}

func (ws *WebSocket) insertMarketArtifact(artifact *datura.Artifact) {
	if ws == nil || ws.tree == nil || artifact == nil {
		return
	}

	packed := artifact.Pack()
	if len(packed) == 0 {
		return
	}

	ws.tree.Insert(artifact.Prefix("role", "timestamp"), packed)
	ws.tree.Insert(artifact.Prefix("role", "scope", "timestamp"), packed)
}

/*
discover parses an instrument frame, persists each online pair's full metadata
(tick_size, qty_min, precision, …) to the tree under instrument/{symbol}, and
subscribes ticker/book/trade for pairs not already in the tree. The instrument
feed is the symbol source of truth — no default list, no symbol cap. The tree is
the only store, so dedup is "is this pair already recorded?", not a side map.
*/
func (ws *WebSocket) discover(data json.RawMessage, frameType string) {
	fresh, err := ws.recordPairs(data, frameType)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken/public: failed to parse instrument frame",
			err,
		))
		return
	}

	if len(fresh) == 0 {
		return
	}

	errnie.Info(fmt.Sprintf("kraken/public: subscribing %d online pairs", len(fresh)))
	ws.subscribeData(fresh)
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
			continue
		}

		if meta.Status != "online" || meta.Symbol == "" {
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

/*
subscribeData subscribes ticker/book/trade for the discovered symbols. Kraken
closes the connection on an oversized subscribe frame and the universe is
hundreds of pairs, so symbols are sent in chunks of market.subscribe_batch.
Writes happen inline on the read-loop goroutine (the sole caller of discover),
keeping a single writer with no locking; each chunk frame is small and fast, so
reads resume between them.
*/
func (ws *WebSocket) subscribeData(symbols []string) {
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
		symbolJSON, _ := sonic.Marshal(symbols[start:end])

		errnie.Error(ws.send([]byte(fmt.Sprintf(
			`{"method": "subscribe","params": {"channel": "ticker", "symbol": %s}}`,
			string(symbolJSON),
		))))

		errnie.Error(ws.send([]byte(fmt.Sprintf(
			`{"method": "subscribe","params": {"channel": "book", "depth": %d, "symbol": %s}}`,
			depth,
			string(symbolJSON),
		))))

		errnie.Error(ws.send([]byte(fmt.Sprintf(
			`{"method": "subscribe","params": {"channel": "trade", "symbol": %s}}`,
			string(symbolJSON),
		))))

		// L3 is authenticated and lives on WebSocketL3URL, not this public socket.
		// The read loop above will persist level3 frames when an authenticated L3
		// connection supplies them; do not send a known-invalid public subscribe.

		if pace := viper.GetDuration("market.subscribe_pace"); pace > 0 && end < len(symbols) {
			time.Sleep(pace)
		}
	}
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

	ws.cancel()
	return err
}

/*
Connect connects to the Kraken public websocket, using Fibonacci backoff.
It will return an error if the connection fails after the max delay.

The delay is calculated using the Fibonacci sequence:
1, 1, 2, 3, 5, 8, 13, 21, 34, 55, 89
*/
func (ws *WebSocket) Connect(endpoint EndpointType, n int) error {
	if n > ws.connectMaxDelay {
		return errnie.Error(errnie.Err(
			errnie.Unknown,
			"kraken/public: connect failed after max delay",
			fmt.Errorf("kraken/public: connect failed after %d seconds", n),
		))
	}

	if ws.isConnected.Load() && ws.conn != nil {
		return nil
	}

	var response *http.Response

	errnie.Info("kraken/public: websocket client dialing")

	ws.conn, response, ws.err = websocket.DefaultDialer.Dial(
		string(endpoint), http.Header{},
	)

	if ws.err == nil && response != nil && response.StatusCode == http.StatusSwitchingProtocols {
		ws.isConnected.Store(true)
		ws.connectDelay = 1
		errnie.Info("kraken/public: websocket connected")

		ws.subscribeAll()

		return nil
	}

	ws.connectDelay = int(
		math.Round((math.Pow(
			math.Phi, float64(n),
		) + math.Pow(
			math.Phi-1, float64(n),
		)) / math.Sqrt(5)),
	)

	time.Sleep(time.Duration(n) * time.Second)

	return ws.Connect(endpoint, ws.connectDelay)
}

/*
subscribeAll subscribes only to the instrument channel. The instrument snapshot
drives ticker/book/trade subscriptions per discovered pair via discover.
*/
func (ws *WebSocket) subscribeAll() {
	// Request the instrument snapshot; recordPairs re-subscribes every online
	// pair from it, which restores data subscriptions after a reconnect.
	errnie.Error(ws.send([]byte(
		`{"method": "subscribe","params": {"channel": "instrument", "snapshot": true}}`,
	)))
}
