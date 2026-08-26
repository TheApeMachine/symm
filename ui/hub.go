package ui

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/backtest"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/telemetry"
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
	ctx             context.Context
	cancel          context.CancelFunc
	err             error
	app             *fiber.App
	listenAddr      string
	workspace       *runtime.Workspace
	desk            *broker.Desk
	playback        Controller
	captures        func() []backtest.CaptureInfo
	diagnostics     DiagnosticsControl
	fluid           *FluidRTC
	maxMessageBytes int
	maxBatchFrames  int
	clients         *sync.Map
	ObserveModule   func(string, time.Duration)
}

type hubClient struct {
	conn     *websocket.Conn
	outbound chan []byte
	done     chan struct{}
}

type diagnosticsToggleRequest struct {
	Enabled bool `json:"enabled"`
}

/*
DiagnosticsControl is the runtime switch the diagnostics page drives, kept as a
narrow seam so the ui package never imports the trader package that owns the
collector.
*/
type DiagnosticsControl interface {
	SetDiagnosticsEnabled(enabled bool)
	DiagnosticsEnabled() bool
}

func (hub *Hub) SetDiagnosticsControl(control DiagnosticsControl) {
	hub.diagnostics = control
}

func (hub *Hub) SetObserver(observer func(string, time.Duration)) {
	hub.ObserveModule = observer

	if hub.fluid != nil {
		hub.fluid.ObserveModule = observer
	}
}

/*
Controller is the replay surface the backtest dashboard commands drive.
*/
type Controller interface {
	Play()
	Pause()
	Seek(at time.Time)
	Select(captureID int64)
	Hindsight(captureID int64)
}

/*
NewHub constructs the dashboard hub from its queue-backed system boundaries and
registers it on the workspace so live frames reach it through Step.
*/
func NewHub(
	ctx context.Context,
	thesis *types.Thesis,
	desk *broker.Desk,
	price *broker.Price,
	balance *broker.Balance,
	workspace *runtime.Workspace,
) *Hub {
	ctx, cancel := context.WithCancel(ctx)
	viper.SetDefault("ui.addr", "127.0.0.1:8765")
	viper.SetDefault("ui.websocket.max_message_bytes", 4*1024*1024)
	// Cap frames assembled into a single FlatBuffers batch so a slow client
	// that lets the queue grow can never drive one builder past the library's
	// 2 GB ceiling before the message-size split runs. The split loop below
	// still trims each actual write to max_message_bytes.
	viper.SetDefault("ui.websocket.max_batch_frames", 256)

	hub := &Hub{
		ctx:        ctx,
		cancel:     cancel,
		listenAddr: viper.GetString("ui.addr"),
		workspace:  workspace,
		desk:       desk,
		app: fiber.New(fiber.Config{
			JSONEncoder:     sonic.Marshal,
			JSONDecoder:     sonic.Unmarshal,
			StrictRouting:   true,
			ReadBufferSize:  4194304,
			WriteBufferSize: 4194304,
		}),
		fluid:           NewFluidRTC(ctx, workspace, "fluid"),
		maxMessageBytes: viper.GetInt("ui.websocket.max_message_bytes"),
		maxBatchFrames:  viper.GetInt("ui.websocket.max_batch_frames"),
		clients:         &sync.Map{},
	}

	if hub.maxBatchFrames < 1 {
		hub.maxBatchFrames = 1
	}

	hub.workspace.WireClass(
		types.ChannelUI,
		"",
		runtime.ServiceUI,
		runtime.DeliveryLatestByKey,
		func(value any) string { return "global" },
		hub.Step,
	)

	hub.app.Use("/ws", func(c fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	hub.app.Get("/backtest/captures", func(c fiber.Ctx) error {
		if hub.captures == nil {
			return c.JSON([]backtest.CaptureInfo{})
		}

		return c.JSON(hub.captures())
	})

	hub.app.Get("/ws", websocket.New(func(conn *websocket.Conn) {
		key := uuid.NewString()
		client := &hubClient{
			conn:     conn,
			outbound: make(chan []byte, hub.maxBatchFrames*2),
			done:     make(chan struct{}),
		}
		hub.clients.Store(key, client)

		defer func() {
			hub.clients.Delete(key)
			close(client.done)
			_ = conn.Conn.Close()
		}()

		go func() {
			for {
				select {
				case <-client.done:
					return
				case payload, ok := <-client.outbound:
					if !ok {
						return
					}

					_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
					writeErr := conn.WriteMessage(websocket.BinaryMessage, payload)

					if writeErr != nil {
						_ = conn.Conn.Close()
						return
					}
				}
			}
		}()

		if balance := hub.balance(); balance != nil {
			wallet := telemetry.EncodeBatch([]*types.UIFrame{balance.Wallet()})
			payload := make([]byte, len(wallet.Bytes))
			copy(payload, wallet.Bytes)
			wallet.Release()

			select {
			case client.outbound <- payload:
			default:
			}
		}

		for {
			messageType, payload, err := conn.Conn.ReadMessage()

			if err != nil {
				return
			}

			if messageType != websocket.TextMessage {
				continue
			}

			hub.handleCommand(payload)
		}
	}))

	hub.registerFluidWebRTC()

	return hub
}

func (hub *Hub) Step(msg any) any {
	started := time.Now()
	defer func() {
		if hub.ObserveModule != nil {
			hub.ObserveModule("hub", time.Since(started))
		}
	}()

	frame, ok := msg.(*types.UIFrame)

	if !ok || frame == nil {
		return nil
	}

	batch := telemetry.EncodeBatch([]*types.UIFrame{frame})
	defer batch.Release()

	payload := make([]byte, len(batch.Bytes))
	copy(payload, batch.Bytes)

	hub.clients.Range(func(key, value any) bool {
		client, valid := value.(*hubClient)

		if !valid || client == nil {
			return true
		}

		select {
		case client.outbound <- payload:
		default:
		}

		return true
	})

	return nil
}

/*
balance returns the shared broker Balance from the workspace, or nil when the
bus is unavailable or the object was never shared.
*/
func (hub *Hub) balance() *broker.Balance {
	if hub.workspace == nil {
		return nil
	}

	shared, _ := hub.workspace.Shared("balance", "")
	balance, _ := shared.(*broker.Balance)

	return balance
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
		if hub.desk == nil {
			errnie.Error(errnie.Err(
				errnie.NotAcceptable,
				"dashboard: broker desk is unavailable for manual exit",
				nil,
			))
			return
		}

		if err := hub.desk.ManualExit(request.Symbol); err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"dashboard: manual exit failed for "+request.Symbol,
				err,
			))
		}
	case "backtest.play":
		if hub.playback != nil {
			hub.playback.Play()
		}
	case "backtest.pause":
		if hub.playback != nil {
			hub.playback.Pause()
		}
	case "backtest.seek":
		if at, err := time.Parse(time.RFC3339Nano, request.At); err == nil && hub.playback != nil {
			hub.playback.Seek(at)
		}
	case "backtest.select":
		if hub.playback != nil {
			hub.playback.Select(request.CaptureID)
		}
	case "backtest.hindsight":
		if hub.playback != nil {
			hub.playback.Hindsight(request.CaptureID)
		}
	}
}

/*
SetPlayback attaches the backtest driver and its capture listing so websocket
commands and the REST route reach it.
*/
func (hub *Hub) SetPlayback(
	controller Controller,
	captures func() []backtest.CaptureInfo,
) {
	if controller != nil {
		hub.playback = controller
	}

	if captures != nil {
		hub.captures = captures
	}
}

/*
Serve listens for dashboard websocket clients.
*/
func (hub *Hub) Run() error {
	if hub.err != nil {
		return hub.err
	}

	return hub.app.Listen(hub.listenAddr)
}

/*
Close shuts down the HTTP server, cancels clients, and waits for ingress drain.
*/
func (hub *Hub) Close() error {
	var err error

	hub.cancel()
	err = errors.Join(err, hub.fluid.Close())

	if hub.app != nil {
		err = hub.app.Shutdown()
	}

	if errors.Is(err, net.ErrClosed) {
		return nil
	}

	return err
}
