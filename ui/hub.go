package ui

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

const writeBudget = 200 * time.Millisecond

/*
Hub owns the dashboard websocket and forwards typed backend frames to clients.
A coalescer always drains Messages (even with no browser) and flushes
latest-by-key snapshots on a fixed cadence so reconnects never replay a backlog.
*/
type Hub struct {
	status     types.Status
	ctx        context.Context
	cancel     context.CancelFunc
	app        *fiber.App
	listenAddr string
	Messages   chan []byte
	balance    *broker.Balance
	ready      func() bool
	live       atomic.Pointer[websocket.Conn]
	latest     sync.Map
}

func NewHub(
	ctx context.Context,
	price *broker.Price,
	balance *broker.Balance,
	thesis *types.Thesis,
	channel chan []byte,
	ready func() bool,
) (*Hub, error) {
	ctx, cancel := context.WithCancel(ctx)

	hub := &Hub{
		status:     types.INITIALIZING,
		ctx:        ctx,
		cancel:     cancel,
		listenAddr: viper.GetString("ui.addr"),
		Messages:   channel,
		app: fiber.New(fiber.Config{
			JSONEncoder: sonic.Marshal,
			JSONDecoder: sonic.Unmarshal,
		}),
		balance: balance,
		ready:   ready,
	}

	hub.app.Use("/ws", func(c fiber.Ctx) error {
		if !hub.open() {
			return fiber.ErrServiceUnavailable
		}

		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	hub.app.Get("/ws", websocket.New(func(conn *websocket.Conn) {
		defer func() {
			hub.live.CompareAndSwap(conn, nil)
			conn.Close()
		}()

		if conn.Conn != nil {
			if hub.balance != nil {
				if frame := hub.balance.Frame(); len(frame) > 0 {
					if err := hub.write(conn, frame); err != nil {
						if hub.clientGone(err) {
							return
						}

						errnie.Error(err)
						return
					}
				}
			}

			hub.latest.Range(func(_, value any) bool {
				frame, ok := value.([]byte)

				if !ok || len(frame) == 0 {
					return true
				}

				if err := hub.write(conn, frame); err != nil {
					if hub.clientGone(err) {
						return false
					}

					errnie.Error(err)
					return false
				}

				return true
			})
		}

		hub.live.Store(conn)
		<-hub.ctx.Done()
	}, websocket.Config{
		EnableCompression: true,
	}))

	return hub, nil
}

func (hub *Hub) open() bool {
	if hub == nil || hub.ready == nil {
		return true
	}

	return hub.ready()
}

func (hub *Hub) Initialize() error {
	if hub == nil {
		return nil
	}

	if hub.status == types.READY {
		return nil
	}

	errnie.Info("initializing UI hub")
	hub.status = types.READY
	go hub.coalesce()
	return nil
}

func (hub *Hub) Serve() error {
	if err := hub.Initialize(); err != nil {
		return err
	}

	return errnie.Error(hub.app.Listen(hub.listenAddr))
}

func (hub *Hub) Close() error {
	if hub.app != nil {
		errnie.Error(hub.app.Shutdown())
	}

	hub.cancel()
	return nil
}

/*
coalesce drains producer frames in bursts and flushes latest-by-key every 16ms.
Write deadlines keep a slow browser from stalling the drain loop for seconds.
*/
func (hub *Hub) coalesce() {
	ticker := time.NewTicker(16 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-hub.ctx.Done():
			return
		case msg := <-hub.Messages:
			hub.ingest(msg)
			hub.drain()
		case <-ticker.C:
			hub.flush()
		}
	}
}

func (hub *Hub) ingest(msg []byte) {
	hub.latest.Store(frameKey(msg), msg)
}

func (hub *Hub) drain() {
	for {
		select {
		case msg := <-hub.Messages:
			hub.ingest(msg)
		default:
			return
		}
	}
}

func (hub *Hub) flush() {
	conn := hub.live.Load()

	if conn == nil || conn.Conn == nil {
		return
	}

	hub.latest.Range(func(key, value any) bool {
		frame, ok := value.([]byte)

		if !ok {
			return true
		}

		if err := hub.write(conn, frame); err != nil {
			if hub.clientGone(err) {
				hub.live.CompareAndSwap(conn, nil)
				return false
			}

			// Keep the snapshot for the next flush — tick frames are small and
			// would otherwise keep updating while mark/PnL frames never land.
			return true
		}

		hub.latest.Delete(key)

		return true
	})
}

/*
write bounds each send so a stalled client cannot freeze the coalescer.
Seed runs before live is published; afterward only coalesce writes.
*/
func (hub *Hub) write(conn *websocket.Conn, frame []byte) error {
	if hub == nil || conn == nil || conn.Conn == nil {
		return errors.New("ui: websocket unavailable")
	}

	if err := conn.Conn.SetWriteDeadline(time.Now().Add(writeBudget)); err != nil {
		return err
	}

	return conn.Conn.WriteMessage(websocket.TextMessage, frame)
}

func frameKey(msg []byte) string {
	// Prefer desk inventory over tick so mark/PnL snapshots are not collapsed
	// under the high-frequency tick key when a payload carries both shapes.
	for _, key := range []string{
		"holdings", "balances", "instruments", "measurements", "manifold",
		"cognition", "forecasts", "decisions", "resonance", "causal",
		"hypotheses", "graphs", "lifecycle", "findings",
		"categories", "stops", "executions", "orders", "tick",
	} {
		needle := []byte(`"` + key + `":`)

		if bytes.Contains(msg, needle) {
			return key
		}
	}

	return "raw"
}

func (hub *Hub) clientGone(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}

	if websocket.IsCloseError(err) {
		return true
	}

	message := err.Error()

	return strings.Contains(message, "unexpected bytes at end of flate stream") ||
		strings.Contains(message, "use of closed network connection") ||
		strings.Contains(message, "broken pipe")
}
