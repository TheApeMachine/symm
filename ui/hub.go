package ui

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
cacheKeys lists every replaceable UI stream retained for reconnect replay.
*/
var cacheKeys = []string{
	"balances", "executions", "instruments", "positions", "tick",
	"holdings", "stops", "measurements", "decisions", "lifecycle", "findings",
	"causal", "resonance", "manifold", "manifold_particles", "manifold_wave",
	"cognition", "diagnostics",
}

var cacheKeySet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(cacheKeys))

	for _, key := range cacheKeys {
		set[key] = struct{}{}
	}

	return set
}()

const clientQueueDepth = 64

/*
cachedFrame is one latest-by-key payload plus the generation that produced it.
*/
type cachedFrame struct {
	generation uint64
	payload    []byte
}

/*
clientSession owns one websocket writer with a bounded latest-by-key queue.
A full queue drops the frame for that client without muting the session.
*/
type clientSession struct {
	conn   *websocket.Conn
	queue  chan cachedFrame
	cancel context.CancelFunc
	done   chan struct{}
}

/*
Hub owns the dashboard websocket and forwards flat JSON frames to clients.
Each client has a bounded latest-by-key writer queue managed by the hub loop.
Publish coalesces replaceable state by key, assigns a generation, and fans out
only to registered clients so one slow peer cannot block the drain.
*/
type Hub struct {
	ctx         context.Context
	cancel      context.CancelFunc
	app         *fiber.App
	listenAddr  string
	Messages    chan []byte
	price       *broker.Price
	balance     *broker.Balance
	cache       sync.Map
	generation  atomic.Uint64
	clients     sync.Map
	dropped     atomic.Uint64
	ingressDone chan struct{}
}

/*
NewHub constructs the dashboard hub from an injected UI config address.
*/
func NewHub(
	ctx context.Context,
	price *broker.Price,
	balance *broker.Balance,
	channel chan []byte,
	cfg config.UIConfig,
) *Hub {
	ctx, cancel := context.WithCancel(ctx)

	addr := cfg.Addr

	if addr == "" {
		addr = "127.0.0.1:8765"
	}

	hub := &Hub{
		ctx:         ctx,
		cancel:      cancel,
		listenAddr:  addr,
		Messages:    channel,
		app: fiber.New(fiber.Config{
			JSONEncoder:     sonic.Marshal,
			JSONDecoder:     sonic.Unmarshal,
			StrictRouting:   true,
			ReadBufferSize:  4 * 1024 * 1024,
			WriteBufferSize: 4 * 1024 * 1024,
		}),
		price:       price,
		balance:     balance,
		ingressDone: make(chan struct{}),
	}

	hub.app.Use("/ws", func(c fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	hub.app.Get("/ws", websocket.New(func(conn *websocket.Conn) {
		session := hub.register(conn)
		defer hub.unregister(session)

		hub.replay(session)
		hub.writeWallet(session)

		for {
			_, payload, err := conn.ReadMessage()

			if err != nil {
				return
			}

			hub.applyClient(payload)
		}
	}))

	go hub.drain()

	return hub
}

/*
Publish coalesces one replaceable frame by its top-level keys, assigns a
generation, and enqueues it for every registered client. Non-replaceable keys
still advance generation so reconnect clients never regress.
*/
func (hub *Hub) Publish(frame []byte) {
	if hub == nil || len(frame) == 0 {
		return
	}

	generation := hub.generation.Add(1)
	retained := hub.retain(generation, frame)

	if len(retained) == 0 {
		hub.fanout(cachedFrame{generation: generation, payload: frame})
		return
	}

	for _, entry := range retained {
		hub.fanout(entry)
	}
}

/*
Dropped returns how many client-queue enqueues were rejected under saturation.
*/
func (hub *Hub) Dropped() uint64 {
	return hub.dropped.Load()
}

func (hub *Hub) drain() {
	defer close(hub.ingressDone)

	for {
		select {
		case <-hub.ctx.Done():
			return
		case msg, ok := <-hub.Messages:
			if !ok {
				return
			}

			hub.Publish(msg)
		}
	}
}

/*
applyClient handles dashboard→backend control frames. Focus updates gate
signal-metric WireMeasurements without affecting balances or decisions.
*/
func (hub *Hub) applyClient(payload []byte) {
	if len(payload) == 0 {
		return
	}

	var inbound struct {
		Type   string `json:"type"`
		Symbol string `json:"symbol"`
	}

	if err := sonic.Unmarshal(payload, &inbound); err != nil {
		return
	}

	if inbound.Type != "focus" {
		return
	}

	types.SetFocus(inbound.Symbol)
}

/*
retain splits a flat frame into latest-by-key cache entries with one top-level
key walk. Probing every cache key with GetBytes re-searched fat manifold
frames ~17× and saturated the hub drain (measured ~1.6ms/frame).
*/
func (hub *Hub) retain(generation uint64, msg []byte) []cachedFrame {
	retained := make([]cachedFrame, 0, 1)

	err := utils.EachKey(msg, func(key string, raw []byte) bool {
		if _, ok := cacheKeySet[key]; !ok || len(raw) == 0 {
			return true
		}

		payload := make([]byte, 0, len(key)+len(raw)+5)
		payload = append(payload, `{"`...)
		payload = append(payload, key...)
		payload = append(payload, `":`...)
		payload = append(payload, raw...)
		payload = append(payload, '}')

		entry := cachedFrame{generation: generation, payload: payload}
		hub.cache.Store(key, entry)
		retained = append(retained, entry)
		return true
	})

	if err != nil {
		return nil
	}

	return retained
}

func (hub *Hub) register(conn *websocket.Conn) *clientSession {
	sessionCtx, cancel := context.WithCancel(hub.ctx)
	session := &clientSession{
		conn:   conn,
		queue:  make(chan cachedFrame, clientQueueDepth),
		cancel: cancel,
		done:   make(chan struct{}),
	}

	hub.clients.Store(session, struct{}{})

	go hub.writeLoop(sessionCtx, session)

	return session
}

func (hub *Hub) unregister(session *clientSession) {
	if session == nil {
		return
	}

	hub.clients.Delete(session)
	session.cancel()
	<-session.done
	_ = session.conn.Close()
}

func (hub *Hub) fanout(frame cachedFrame) {
	hub.clients.Range(func(key, _ any) bool {
		session, ok := key.(*clientSession)

		if !ok || session == nil {
			return true
		}

		select {
		case session.queue <- frame:
		default:
			// Drop the frame for this client only. Cancelling the write loop
			// while leaving the socket open freezes the UI until a manual
			// refresh; saturation must not mute a live session.
			total := hub.dropped.Add(1)

			if total == 1 || total%64 == 0 {
				errnie.Error(errnie.Err(
					errnie.TooManyRequests,
					"ui hub: client queue saturated; dropped frames",
					nil,
				))
			}
		}

		return true
	})
}

func (hub *Hub) replay(session *clientSession) {
	if session == nil {
		return
	}

	snapshot := make([]cachedFrame, 0, len(cacheKeys))
	ceiling := hub.generation.Load()

	for _, key := range cacheKeys {
		value, ok := hub.cache.Load(key)

		if !ok {
			continue
		}

		entry, ok := value.(cachedFrame)

		if !ok || len(entry.payload) == 0 || entry.generation > ceiling {
			continue
		}

		snapshot = append(snapshot, entry)
	}

	for _, entry := range snapshot {
		select {
		case session.queue <- entry:
		default:
			hub.dropped.Add(1)
		}
	}
}

/*
writeWallet pushes a fresh Balance.Frame so wallet/holdings are not stranded
when earlier publishes were dropped under saturation. With a session, the
frame is also queued for that client; without one, only the cache updates.
*/
func (hub *Hub) writeWallet(session *clientSession) {
	if hub.balance == nil {
		return
	}

	frame, err := hub.balance.Frame()

	if err != nil {
		errnie.Error(err)

		return
	}

	if len(frame) == 0 {
		return
	}

	generation := hub.generation.Add(1)
	retained := hub.retain(generation, frame)

	if session == nil {
		return
	}

	for _, entry := range retained {
		select {
		case session.queue <- entry:
		default:
			hub.dropped.Add(1)
		}
	}
}

/*
Cached returns the latest retained payload for one cache key after drain.
*/
func (hub *Hub) Cached(key string) []byte {
	if hub == nil {
		return nil
	}

	value, ok := hub.cache.Load(key)

	if !ok {
		return nil
	}

	entry, ok := value.(cachedFrame)

	if !ok {
		return nil
	}

	return entry.payload
}

func (hub *Hub) writeLoop(ctx context.Context, session *clientSession) {
	defer close(session.done)

	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-session.queue:
			if !ok {
				return
			}

			if len(frame.payload) == 0 {
				continue
			}

			if err := hub.writeMessage(session.conn, frame.payload); err != nil {
				session.cancel()
				return
			}
		}
	}
}

func (hub *Hub) writeMessage(conn *websocket.Conn, msg []byte) error {
	if conn == nil || conn.Conn == nil || len(msg) == 0 {
		return nil
	}

	if err := conn.Conn.WriteMessage(websocket.TextMessage, msg); err != nil {
		for _, closeError := range []error{
			syscall.EPIPE,
			syscall.ECONNRESET,
			io.EOF,
			io.ErrClosedPipe,
		} {
			if errors.Is(err, closeError) {
				return err
			}
		}

		return err
	}

	return nil
}

/*
Serve listens for dashboard websocket clients.
*/
func (hub *Hub) Serve() error {
	return hub.app.Listen(hub.listenAddr)
}

/*
Close shuts down the HTTP server, cancels clients, and waits for ingress drain.
*/
func (hub *Hub) Close() error {
	var err error

	if hub.app != nil {
		err = hub.app.Shutdown()
	}

	hub.cancel()

	hub.clients.Range(func(key, _ any) bool {
		if session, ok := key.(*clientSession); ok && session != nil {
			session.cancel()
		}

		return true
	})

	<-hub.ingressDone

	return err
}

