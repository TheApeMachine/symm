package ui

import (
	"context"
	"errors"
	"net"
	neturl "net/url"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/google/uuid"
	"github.com/spf13/viper"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

/*
Hub owns the dashboard websocket and broadcasts schema-tagged binary frames.
It is an ordinary Workspace stage: it registers to ChannelUI through NewHub,
and the Workspace drives every outbound write through Step. Inbound commands
arrive over the same socket and are handled directly by the connection's
handler goroutine, so there are no per-client writer or reader goroutines.
*/
type Hub struct {
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	app        *fiber.App
	listenAddr string
	clients    *sync.Map
}

type Client struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

/*
NewHub constructs the dashboard hub from its queue-backed system boundaries and
registers it on the workspace so live frames reach it through Step.
*/
func NewHub(ctx context.Context) *Hub {
	ctx, cancel := context.WithCancel(ctx)
	viper.SetDefault("ui.addr", "127.0.0.1:8765")
	viper.SetDefault("ui.websocket.max_message_bytes", 4*1024*1024)

	hub := &Hub{
		ctx:        ctx,
		cancel:     cancel,
		listenAddr: viper.GetString("ui.addr"),
		app: fiber.New(fiber.Config{
			JSONEncoder:     sonic.Marshal,
			JSONDecoder:     sonic.Unmarshal,
			StrictRouting:   true,
			ReadBufferSize:  4194304,
			WriteBufferSize: 4194304,
		}),
		clients: &sync.Map{},
	}

	// The dashboard is a separate origin from the hub (vite dev server on
	// :3000 vs. the hub on :8765). The REST capture listing is fetched with a
	// plain cross-origin GET, so the browser blocks the response without an
	// Access-Control-Allow-Origin header. Permit loopback origins on any port
	// so a locally-served dashboard can always read it without opening CORS to
	// arbitrary remote origins.
	hub.app.Use(cors.New(cors.Config{
		AllowOriginsFunc: func(origin string) bool {
			parsed, err := neturl.Parse(origin)

			if err != nil {
				return false
			}

			host := parsed.Hostname()

			return host == "localhost" || host == "127.0.0.1" || host == "::1"
		},
		AllowHeaders: []string{"Content-Type"},
	}))

	hub.app.Use("/ws", func(c fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	hub.app.Get("/trades", func(c fiber.Ctx) error {
		// No broker.PositionStore is wired to the hub yet — that lands with
		// the desk/broker migration, out of scope here. Until then this
		// route reports "no trades" rather than fail the request.
		return c.JSON([]*wire.PositionT{})
	})

	hub.app.Get("/ws", websocket.New(func(conn *websocket.Conn) {
		key := uuid.NewString()
		hub.clients.Store(key, &Client{
			conn: conn,
		})

		defer func() {
			hub.clients.Delete(key)
			conn.Conn.Close()
		}()

		for {
			select {
			case <-ctx.Done():
				return
			default:
				messageType, payload, err := conn.Conn.ReadMessage()

				if err != nil {
					return
				}

				if messageType != websocket.TextMessage {
					continue
				}

				hub.handleCommand(payload)
			}
		}
	}, websocket.Config{
		Origins: []string{"*"},
	}))

	hub.registerFluidWebRTC()

	return hub
}

/*
Step is the Ticker/Trade/Level3 Observe boundary's UI publisher (README §12,
§18): it witnesses the Envelope committed by the previous barrier and
broadcasts it as-is — the same exported state the backend is carrying,
converted to FlatBuffers and never reshaped into a curated per-consumer
frame. It never mutates the Envelope and its return value is discarded by the
Observe HandlerGroup, so a disconnected or slow UI cannot change what the
system believes.
*/
func (hub *Hub) Step(envelope *types.Envelope) *types.Envelope {
	payload := envelope.EncodeBytes()

	hub.clients.Range(func(key, value any) bool {
		client, valid := value.(*Client)

		if !valid || client == nil {
			return true
		}

		client.mu.Lock()
		_ = client.conn.WriteMessage(websocket.BinaryMessage, payload)
		client.mu.Unlock()

		return true
	})

	return envelope
}

func (hub *Hub) Name() string { return "hub" }
func (hub *Hub) Error() error { return hub.err }

/*
handleCommand dispatches one inbound JSON command from the dashboard socket.
*/
func (hub *Hub) handleCommand(payload []byte) {
	var request struct {
		Type      string `json:"type"`
		Symbol    string `json:"symbol"`
		At        string `json:"at"`
		CaptureID int64  `json:"captureId"`
	}

	if err := sonic.Unmarshal(payload, &request); err != nil {
		return
	}

	switch request.Type {
	case "focus":
		types.SetFocus(request.Symbol)
	case "position.exit":
	}
}

/*
Serve listens for dashboard websocket clients.
*/
func (hub *Hub) Run() error {
	return hub.app.Listen(":8765")
}

/*
Close shuts down the HTTP server, cancels clients, and waits for ingress drain.
*/
func (hub *Hub) Close() error {
	var err error

	hub.cancel()

	if hub.app != nil {
		err = hub.app.Shutdown()
	}

	if errors.Is(err, net.ErrClosed) {
		return nil
	}

	return err
}
