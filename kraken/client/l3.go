package client

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/core"
	"github.com/theapemachine/symm/kraken/level3"
	"github.com/theapemachine/symm/runstats"
)

// l3SubscribeChunk bounds how many symbols go in one level3 subscribe frame.
// level3 at depth 10 charges +5 per symbol on the subscribe-rate counter, so
// small chunks keep a (re)connect flush under the standard-tier budget.
const l3SubscribeChunk = 20

// l3Depth is the shallow book depth requested per symbol. Spoof walls sit near
// the touch, so depth 10 is sufficient and cheapest on the rate counter.
const l3Depth = 10

/*
L3Client maintains an authenticated Kraken WebSocket v2 "level3" session. It
mirrors PrivateClient's single-writer + supervisor + token-watcher discipline
(see private.go), but its only job is to parse the order-by-order feed into
neutral level3.Order events and broadcast them on the "level3" group, where the
toxicity signal joins them against the public trade tape. It owns no orders and
sends nothing but subscribe and ping frames.

It subscribes to whatever symbols the rest of the system has requested on the
"subscriptions" group (the same set the public client streams), so L3 covers
exactly the symbols under active watch. When no credentials are configured this
client is never registered, every "level3" event is therefore absent, and the
toxicity tracker falls back to the public L2 book for every symbol.
*/
type L3Client struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	url         string
	apiKey      string
	apiSecret   string

	connMu sync.RWMutex
	conn   *websocket.Conn
	token  atomic.Pointer[kraken.Token]

	subMu     sync.Mutex
	requested map[string]bool

	writeCh chan writeJob
}

func NewL3Client(
	ctx context.Context,
	pool *qpool.Q,
	url string,
	apiKey string,
	apiSecret string,
) *L3Client {
	ctx, cancel := context.WithCancel(ctx)

	client := &L3Client{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		url:         url,
		apiKey:      apiKey,
		apiSecret:   apiSecret,
		requested:   make(map[string]bool),
		writeCh:     make(chan writeJob, 256),
	}

	client.broadcasts["level3"] = pool.CreateBroadcastGroup("level3", 10*time.Millisecond)
	client.subscribers["subscriptions"] = pool.CreateBroadcastGroup("subscriptions", 10*time.Millisecond).
		Subscribe("l3:subscriptions", 128)

	return client
}

func (l3 *L3Client) Start() error {
	return nil
}

func (l3 *L3Client) State() engine.State {
	return engine.READY
}

func (l3 *L3Client) Tick() error {
	var workers sync.WaitGroup

	workers.Go(func() { l3.runWriter() })
	workers.Go(func() { l3.runSubscriptions() })
	workers.Go(func() { l3.runSupervisor() })

	<-l3.ctx.Done()
	workers.Wait()

	return l3.ctx.Err()
}

func (l3 *L3Client) runSupervisor() {
	backoff := privateReconnectBackoffMs
	connected := false

	for l3.ctx.Err() == nil {
		if err := l3.connectOnce(); err != nil {
			errnie.Error(err)
			l3.sleepBackoff(&backoff)

			continue
		}

		if connected {
			runstats.WSReconnect()
			logEvent("l3_ws_reconnected", map[string]any{"backoff_ms": backoff})
		} else {
			runstats.WSConnect()
			logEvent("l3_ws_connected", nil)
			connected = true
		}

		backoff = privateReconnectBackoffMs
		l3.readLoop()
		l3.cycleConnection()
		l3.sleepBackoff(&backoff)
	}
}

func (l3 *L3Client) sleepBackoff(backoff *int) {
	timer := time.NewTimer(time.Duration(*backoff) * time.Millisecond)
	defer timer.Stop()

	select {
	case <-l3.ctx.Done():
		return
	case <-timer.C:
	}

	*backoff *= 2

	if *backoff > privateMaxBackoffMs {
		*backoff = privateMaxBackoffMs
	}
}

func (l3 *L3Client) connectOnce() error {
	if l3.apiKey == "" || l3.apiSecret == "" {
		return errnie.Error(fmt.Errorf("l3 client requires API credentials"))
	}

	token, err := kraken.NewToken(l3.apiKey, l3.apiSecret)

	if err != nil {
		return err
	}

	l3.token.Store(token)

	conn, _, err := websocket.DefaultDialer.DialContext(l3.ctx, l3.url, nil)

	if err != nil {
		return err
	}

	conn.SetReadLimit(1 << 20)
	_ = conn.SetReadDeadline(time.Now().Add(privateReadDeadline))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(privateReadDeadline))
	})

	l3.connMu.Lock()
	l3.conn = conn
	l3.connMu.Unlock()

	// Re-subscribe everything requested so a reconnect restores full coverage.
	l3.flushSubscriptions()

	return nil
}

func (l3 *L3Client) cycleConnection() {
	l3.connMu.Lock()
	conn := l3.conn
	l3.conn = nil
	l3.connMu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}
}

func (l3 *L3Client) readLoop() {
	conn := l3.currentConn()

	if conn == nil {
		return
	}

	stop := make(chan struct{})
	defer close(stop)

	go l3.runPinger(stop)
	go l3.runTokenWatcher(stop)

	for l3.ctx.Err() == nil {
		_, payload, err := conn.ReadMessage()

		if err != nil {
			errnie.Error(err)

			return
		}

		_ = conn.SetReadDeadline(time.Now().Add(privateReadDeadline))
		l3.routeFrame(payload)
	}
}

func (l3 *L3Client) routeFrame(payload []byte) {
	orders, ok := level3.ParseOrders(payload, time.Now())

	if !ok {
		return
	}

	l3.broadcasts["level3"].Send(&qpool.QValue[any]{Value: orders})
}

func (l3 *L3Client) currentConn() *websocket.Conn {
	l3.connMu.RLock()
	defer l3.connMu.RUnlock()

	return l3.conn
}

func (l3 *L3Client) runPinger(stop <-chan struct{}) {
	ticker := time.NewTicker(privatePingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-l3.ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
			l3.postJob(writeJob{kind: writePing})
		}
	}
}

func (l3 *L3Client) runTokenWatcher(stop <-chan struct{}) {
	ticker := time.NewTicker(privateTokenCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-l3.ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
			token := l3.token.Load()

			if token == nil || !token.Expired() {
				continue
			}

			if err := token.Refresh(l3.apiKey, l3.apiSecret); err != nil {
				errnie.Error(err)
				runstats.TokenRefresh(false)

				continue
			}

			runstats.TokenRefresh(true)
		}
	}
}

/*
runSubscriptions drains the shared "subscriptions" group. Every symbol the
system asks the public client to stream is also subscribed on L3 (deduped), so
the toxicity tracker sees order-by-order flow for exactly the watched set.
*/
func (l3 *L3Client) runSubscriptions() {
	incoming := l3.subscribers["subscriptions"].Incoming

	for {
		select {
		case <-l3.ctx.Done():
			return
		case value, ok := <-incoming:
			if !ok {
				return
			}

			symbols, ok := value.Value.([]string)

			if !ok {
				continue
			}

			fresh := l3.markRequested(symbols)

			if len(fresh) > 0 {
				l3.subscribe(fresh)
			}
		}
	}
}

// markRequested records symbols and returns only the ones newly added.
func (l3 *L3Client) markRequested(symbols []string) []string {
	l3.subMu.Lock()
	defer l3.subMu.Unlock()

	fresh := make([]string, 0, len(symbols))

	for _, symbol := range symbols {
		if symbol == "" || l3.requested[symbol] {
			continue
		}

		l3.requested[symbol] = true
		fresh = append(fresh, symbol)
	}

	return fresh
}

func (l3 *L3Client) flushSubscriptions() {
	l3.subMu.Lock()
	symbols := make([]string, 0, len(l3.requested))

	for symbol := range l3.requested {
		symbols = append(symbols, symbol)
	}

	l3.subMu.Unlock()

	l3.subscribe(symbols)
}

// subscribe sends level3 subscribe frames in bounded chunks. Best-effort: when
// the socket is down, send returns an error and the symbols are re-flushed on
// the next connect.
func (l3 *L3Client) subscribe(symbols []string) {
	token := ""

	if t := l3.token.Load(); t != nil {
		token = t.Value()
	}

	if token == "" {
		return
	}

	for start := 0; start < len(symbols); start += l3SubscribeChunk {
		end := min(start+l3SubscribeChunk, len(symbols))

		if err := l3.send(map[string]any{
			"method": "subscribe",
			"params": map[string]any{
				"channel":  core.ChannelLevel3,
				"symbol":   symbols[start:end],
				"depth":    l3Depth,
				"snapshot": true,
				"token":    token,
			},
		}); err != nil {
			errnie.Error(err)

			return
		}
	}
}

func (l3 *L3Client) runWriter() {
	for {
		select {
		case <-l3.ctx.Done():
			return
		case job := <-l3.writeCh:
			err := l3.executeJob(job)

			if job.done != nil {
				select {
				case job.done <- err:
				default:
				}
			}
		}
	}
}

func (l3 *L3Client) executeJob(job writeJob) error {
	conn := l3.currentConn()

	if conn == nil {
		return fmt.Errorf("no live websocket")
	}

	if err := conn.SetWriteDeadline(time.Now().Add(privateWriteDeadline)); err != nil {
		return err
	}

	switch job.kind {
	case writePing:
		return conn.WriteMessage(websocket.PingMessage, nil)
	case writeJSON:
		return conn.WriteJSON(job.payload)
	}

	return fmt.Errorf("unknown write job kind: %d", job.kind)
}

func (l3 *L3Client) postJob(job writeJob) {
	select {
	case l3.writeCh <- job:
	default:
	}
}

func (l3 *L3Client) send(payload any) error {
	done := make(chan error, 1)
	job := writeJob{kind: writeJSON, payload: payload, done: done}

	select {
	case <-l3.ctx.Done():
		return l3.ctx.Err()
	case l3.writeCh <- job:
	}

	select {
	case <-l3.ctx.Done():
		return l3.ctx.Err()
	case err := <-done:
		return err
	}
}

func (l3 *L3Client) Close() error {
	l3.cancel()
	l3.cycleConnection()

	return nil
}
