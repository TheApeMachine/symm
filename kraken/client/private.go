package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/phuslu/log"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/core"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/order"
	"github.com/theapemachine/symm/runstats"
)

/*
logEvent emits one structured log line so private-client lifecycle is
searchable in the run log alongside the trader's audit lines. errnie's
Info path also feeds the same file writer, so the lines interleave with
the rest of the audit stream.
*/
func logEvent(event string, fields map[string]any) {
	if fields == nil {
		fields = map[string]any{}
	}

	fields["event"] = event
	fields["ts"] = time.Now().UTC().Format(time.RFC3339Nano)

	payload, err := json.Marshal(fields)

	if err != nil {
		log.Error().Err(err).Str("event", event).Msg("private: log marshal")
		return
	}

	errnie.Info(string(payload))
}

const (
	privateWriteDeadline      = 5 * time.Second
	privateReadDeadline       = 90 * time.Second
	privatePingInterval       = 25 * time.Second
	privateReconnectBackoffMs = 500
	privateMaxBackoffMs       = 30_000
	privateTokenCheckInterval = 30 * time.Second
)

/*
PrivateClient maintains an authenticated Kraken WebSocket v2 session.

Single writer invariant. The connection has exactly one writer goroutine
(runWriter). The supervisor, the pinger, the token watcher, and the order
dispatcher never call WriteJSON / WriteMessage on the conn directly —
they post jobs onto writeCh. This is what makes the conn safe under
concurrency: fasthttp/websocket explicitly forbids concurrent writes,
and a stuck WriteJSON call would otherwise pin the mutex used to
serialize the previous design's writes. The single writer also provides
the natural place to apply SetWriteDeadline on every frame.

Ack routing. Frames whose top-level "method" matches the trading methods
(add_order, cancel_order, edit_order) are decoded via order.ParseAck and
broadcast on the "order_acks" group so the caller (the trader, via
SubmitLive's req_id) can correlate. Without this, ack-based exchange
order IDs never reach the wallet and Cancel paths.
*/
type PrivateClient struct {
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
	epoch  atomic.Uint64
	token  atomic.Pointer[kraken.Token]

	// writeCh is the single serialized path to the conn. anything that
	// wants to send a frame posts onto it; runWriter is the only thing
	// that calls WriteJSON / WriteMessage / SetWriteDeadline.
	writeCh chan writeJob
}

type writeJobKind uint8

const (
	writeJSON writeJobKind = iota
	writePing
)

type writeJob struct {
	kind    writeJobKind
	payload any
	done    chan error
}

/*
NewPrivateClient creates a private websocket client bound to parent context cancellation.
*/
func NewPrivateClient(
	ctx context.Context,
	pool *qpool.Q,
	url string,
	apiKey string,
	apiSecret string,
) *PrivateClient {
	ctx, cancel := context.WithCancel(ctx)

	client := &PrivateClient{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		url:         url,
		apiKey:      apiKey,
		apiSecret:   apiSecret,
		writeCh:     make(chan writeJob, 256),
	}

	client.broadcasts["executions"] = pool.CreateBroadcastGroup("executions", 10*time.Millisecond)
	client.broadcasts["order_acks"] = pool.CreateBroadcastGroup("order_acks", 10*time.Millisecond)
	client.subscribers["orders"] = pool.CreateBroadcastGroup("orders", 10*time.Millisecond).Subscribe("private:orders", 128)

	if errnie.Error(errnie.Require(map[string]any{
		"ctx":         ctx,
		"cancel":      cancel,
		"pool":        pool,
		"broadcasts":  client.broadcasts,
		"subscribers": client.subscribers,
		"url":         url,
		"apiKey":      apiKey,
		"apiSecret":   apiSecret,
	})) != nil {
		return nil
	}

	return client
}

func (privateClient *PrivateClient) Start() error {
	return nil
}

func (privateClient *PrivateClient) State() engine.State {
	return engine.READY
}

/*
Tick drives the supervisor, the writer, and the order pump. It returns
when the parent context is cancelled.
*/
func (privateClient *PrivateClient) Tick() error {
	var workers sync.WaitGroup

	workers.Go(func() { privateClient.runWriter() })
	workers.Go(func() { privateClient.runOrderPump() })
	workers.Go(func() { privateClient.runSupervisor() })

	<-privateClient.ctx.Done()
	workers.Wait()
	return privateClient.ctx.Err()
}

/*
runSupervisor maintains a live, authenticated connection. Each iteration
fetches a fresh token, dials, subscribes, and blocks until the read loop
returns; an exponential backoff caps reconnect frequency.
*/
func (privateClient *PrivateClient) runSupervisor() {
	backoff := privateReconnectBackoffMs
	connected := false

	for privateClient.ctx.Err() == nil {
		if err := privateClient.connectOnce(); err != nil {
			errnie.Error(err)
			privateClient.sleepBackoff(&backoff)
			continue
		}

		if connected {
			runstats.WSReconnect()
			logEvent("private_ws_reconnected", map[string]any{
				"backoff_ms": backoff,
			})
		} else {
			runstats.WSConnect()
			logEvent("private_ws_connected", nil)
			connected = true
		}

		backoff = privateReconnectBackoffMs
		privateClient.readLoop()
		privateClient.cycleConnection()
		privateClient.sleepBackoff(&backoff)
	}
}

func (privateClient *PrivateClient) sleepBackoff(backoff *int) {
	timer := time.NewTimer(time.Duration(*backoff) * time.Millisecond)
	defer timer.Stop()

	select {
	case <-privateClient.ctx.Done():
		return
	case <-timer.C:
	}

	*backoff *= 2

	if *backoff > privateMaxBackoffMs {
		*backoff = privateMaxBackoffMs
	}
}

func (privateClient *PrivateClient) connectOnce() error {
	if privateClient.apiKey == "" || privateClient.apiSecret == "" {
		return errnie.Error(fmt.Errorf("private client requires API credentials"))
	}

	token, err := kraken.NewToken(privateClient.apiKey, privateClient.apiSecret)

	if err != nil {
		return err
	}

	privateClient.token.Store(token)

	conn, _, err := websocket.DefaultDialer.DialContext(
		privateClient.ctx, privateClient.url, nil,
	)

	if err != nil {
		return err
	}

	conn.SetReadLimit(1 << 20)
	_ = conn.SetReadDeadline(time.Now().Add(privateReadDeadline))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(privateReadDeadline))
	})

	privateClient.connMu.Lock()
	privateClient.conn = conn
	privateClient.connMu.Unlock()
	privateClient.epoch.Add(1)

	// Subscribe via the single writer so the deadline / serialization
	// guarantees hold even for the very first frame.
	if err := privateClient.send(map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel":     core.ChannelExecutions,
			"token":       token.Value(),
			"snap_orders": true,
		},
	}); err != nil {
		privateClient.cycleConnection()
		return err
	}

	return nil
}

func (privateClient *PrivateClient) cycleConnection() {
	privateClient.connMu.Lock()
	conn := privateClient.conn
	privateClient.conn = nil
	privateClient.connMu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}
}

/*
readLoop returns when the websocket fails or the parent context is
cancelled. Frames are routed: executions → fills broadcast; ack methods
(add_order / cancel_order / edit_order) → order_acks broadcast. Anything
else is dropped silently — the reader is not authoritative for channel
discovery.
*/
func (privateClient *PrivateClient) readLoop() {
	conn := privateClient.currentConn()

	if conn == nil {
		return
	}

	stop := make(chan struct{})
	defer close(stop)

	go privateClient.runPinger(stop)
	go privateClient.runTokenWatcher(stop)

	for privateClient.ctx.Err() == nil {
		_, payload, err := conn.ReadMessage()

		if err != nil {
			errnie.Error(err)
			return
		}

		_ = conn.SetReadDeadline(time.Now().Add(privateReadDeadline))
		privateClient.routeFrame(payload)
	}
}

func (privateClient *PrivateClient) routeFrame(payload []byte) {
	channel, channelErr := market.ChannelName(payload)

	if channelErr == nil && channel == core.ChannelExecutions {
		privateClient.handleExecutions(payload)
		return
	}

	// Non-channel frames carry "method" — these are trading acks. We
	// peek at the method field directly so we don't pay the ParseAck
	// cost for every heartbeat or status frame.
	var head struct {
		Method string `json:"method"`
	}

	if err := json.Unmarshal(payload, &head); err != nil {
		return
	}

	switch head.Method {
	case order.MethodAddOrder, order.MethodAmendOrder, "edit_order", order.MethodCancelOrder:
		ack, err := order.ParseAck(payload)

		if err != nil {
			errnie.Error(err)
			return
		}

		privateClient.broadcasts["order_acks"].Send(&qpool.QValue[any]{Value: ack})
	}
}

func (privateClient *PrivateClient) handleExecutions(payload []byte) {
	fills, err := order.ParseExecutionFills(payload)

	if err != nil {
		errnie.Error(err)
		return
	}

	executions := privateClient.broadcasts["executions"]

	for _, fill := range fills {
		executions.Send(&qpool.QValue[any]{Value: fill})
	}
}

func (privateClient *PrivateClient) currentConn() *websocket.Conn {
	privateClient.connMu.RLock()
	defer privateClient.connMu.RUnlock()

	return privateClient.conn
}

func (privateClient *PrivateClient) runPinger(stop <-chan struct{}) {
	ticker := time.NewTicker(privatePingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-privateClient.ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
			// Posts a ping job onto the writer; never touches the conn
			// directly. If the writer or the conn is gone, the job is
			// dropped and the read deadline will close the session.
			privateClient.postJob(writeJob{kind: writePing})
		}
	}
}

/*
runTokenWatcher refreshes the auth token before its server-declared lifetime
elapses. A fresh token does not, by itself, force a reconnect; new requests
pick it up via the atomic pointer, and the connection stays open until
Kraken's read deadline or a network event ends it.
*/
func (privateClient *PrivateClient) runTokenWatcher(stop <-chan struct{}) {
	ticker := time.NewTicker(privateTokenCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-privateClient.ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
			token := privateClient.token.Load()

			if token == nil || !token.Expired() {
				continue
			}

			if err := token.Refresh(privateClient.apiKey, privateClient.apiSecret); err != nil {
				errnie.Error(err)
				runstats.TokenRefresh(false)
				logEvent("token_refresh_failed", map[string]any{
					"error": err.Error(),
				})

				continue
			}

			runstats.TokenRefresh(true)
			logEvent("token_refreshed", nil)
		}
	}
}

/*
runWriter is the only goroutine that calls conn.WriteJSON /
WriteMessage. Frames are received on writeCh; the writer applies
SetWriteDeadline, writes, and signals the per-job done channel when one
was provided.
*/
func (privateClient *PrivateClient) runWriter() {
	for {
		select {
		case <-privateClient.ctx.Done():
			return
		case job := <-privateClient.writeCh:
			err := privateClient.executeJob(job)

			if job.done != nil {
				select {
				case job.done <- err:
				default:
				}
			}
		}
	}
}

func (privateClient *PrivateClient) executeJob(job writeJob) error {
	conn := privateClient.currentConn()

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

func (privateClient *PrivateClient) postJob(job writeJob) {
	select {
	case privateClient.writeCh <- job:
	default:
	}
}

/*
send posts one JSON frame onto the writer and waits for completion or
context cancellation. Used by the supervisor's subscribe step.
*/
func (privateClient *PrivateClient) send(payload any) error {
	done := make(chan error, 1)
	job := writeJob{kind: writeJSON, payload: payload, done: done}

	select {
	case <-privateClient.ctx.Done():
		return privateClient.ctx.Err()
	case privateClient.writeCh <- job:
	}

	select {
	case <-privateClient.ctx.Done():
		return privateClient.ctx.Err()
	case err := <-done:
		return err
	}
}

/*
runOrderPump drains the orders subscription and forwards each request to
the writer. Tokens are stamped just before send so a token refresh
landed while the request was queued takes effect.
*/
func (privateClient *PrivateClient) runOrderPump() {
	orders := privateClient.subscribers["orders"].Incoming

	for {
		select {
		case <-privateClient.ctx.Done():
			return
		case value, ok := <-orders:
			if !ok {
				return
			}

			payload := privateClient.stampToken(value.Value)

			if payload == nil {
				continue
			}

			if err := privateClient.send(payload); err != nil {
				errnie.Error(err)
			}
		}
	}
}

func (privateClient *PrivateClient) stampToken(raw any) any {
	token := ""

	if t := privateClient.token.Load(); t != nil {
		token = t.Value()
	}

	switch request := raw.(type) {
	case order.Request:
		request.Params.Token = token
		return request
	case order.CancelRequest:
		request.Params.Token = token
		return request
	}

	errnie.Error(fmt.Errorf("invalid order request: %T", raw))
	return nil
}

/*
Connect performs one immediate dial. Retained for tests and callers that
need to surface the first connect error directly; the supervisor handles
all subsequent reconnects transparently.
*/
func (privateClient *PrivateClient) Connect() error {
	return privateClient.connectOnce()
}

/*
Close cancels the session context and closes the underlying socket.
*/
func (privateClient *PrivateClient) Close() error {
	privateClient.cancel()
	privateClient.cycleConnection()

	return nil
}
