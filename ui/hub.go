package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/bytedance/sonic"
	fastwebsocket "github.com/fasthttp/websocket"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/backtest"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/telemetry"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

/*
consumerSeq numbers each websocket client's private consumer cursor so clients
drain the shared lock-free transport independently without sharing a cursor.
*/
var consumerSeq atomic.Uint64

func consumerIDFor() string {
	return "dashboard-" + fmt.Sprint(consumerSeq.Add(1))
}

/*
Hub owns the dashboard websocket and broadcasts schema-tagged binary frames.
Each client has a bounded writer queue; a peer that cannot keep up is closed
with an observable error so it cannot block market telemetry for every peer.
*/
type Hub struct {
	ctx              context.Context
	cancel           context.CancelFunc
	err              error
	app              *fiber.App
	listenAddr       string
	thesis           *types.Thesis
	desk             *broker.Desk
	price            *broker.Price
	balance          *broker.Balance
	playback         playback
	captures         func() []backtest.CaptureInfo
	fluid            *FluidRTC
	maxMessageBytes  int
	clientQueueLimit uint64
	writeWindow      uint64
}

/*
playback is the backtest control plane exposed to dashboard commands.
*/
type playback interface {
	Play()
	Pause()
	Seek(at time.Time)
	Select(captureID int64)
	Hindsight(captureID int64)
}

/*
NewHub constructs the dashboard hub from its queue-backed system boundaries.
*/
func NewHub(
	ctx context.Context,
	thesis *types.Thesis,
	desk *broker.Desk,
	price *broker.Price,
	balance *broker.Balance,
	manifold *transport.MapReduce[types.FluidFrame],
) *Hub {
	ctx, cancel := context.WithCancel(ctx)
	viper.SetDefault("ui.websocket.max_message_bytes", 4*1024*1024)
	viper.SetDefault("ui.websocket.client_queue_frames", 16384)
	viper.SetDefault("ui.websocket.write_window", 4)

	hub := &Hub{
		ctx:        ctx,
		cancel:     cancel,
		listenAddr: "127.0.0.1:8765",
		thesis:     thesis,
		desk:       desk,
		app: fiber.New(fiber.Config{
			JSONEncoder:     sonic.Marshal,
			JSONDecoder:     sonic.Unmarshal,
			StrictRouting:   true,
			ReadBufferSize:  4 * 1024 * 1024,
			WriteBufferSize: 4 * 1024 * 1024,
		}),
		price:            price,
		balance:          balance,
		fluid:            NewFluidRTC(ctx, manifold, "fluid"),
		maxMessageBytes:  viper.GetInt("ui.websocket.max_message_bytes"),
		clientQueueLimit: uint64(viper.GetInt("ui.websocket.client_queue_frames")),
		writeWindow:      uint64(viper.GetInt("ui.websocket.write_window")),
	}

	if hub.clientQueueLimit == 0 || hub.writeWindow == 0 {
		hub.err = fmt.Errorf(
			"dashboard: client_queue_frames and write_window must be positive",
		)
	}

	hub.app.Use("/ws", func(c fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	hub.app.Get("/backtest/captures", func(c fiber.Ctx) error {
		if hub.captures == nil {
			return c.JSON([]any{})
		}

		return c.JSON(hub.captures())
	})

	hub.app.Get("/ws", websocket.New(func(conn *websocket.Conn) {
		if hub.err != nil {
			return
		}

		var ui *transport.MapReduce[*types.UIFrame]

		if hub.thesis != nil {
			ui = hub.thesis.UI()
		}

		if ui == nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"dashboard: UI telemetry transport is unavailable",
				nil,
			))
			return
		}

		consumerID := consumerIDFor()

		// Each client drains the lock-free UI transport under its own consumer
		// cursor. MapReduce fans every pushed frame out to every registered
		// consumer, so each client receives each frame without any shared
		// broadcast or client-fan-out machinery on the hub.
		ui.RegisterBounded(consumerID, hub.clientQueueLimit)
		defer ui.Unregister(consumerID)
		initialFrames := make([]*types.UIFrame, 0, 3)

		if hub.balance != nil {
			initialFrames = append(initialFrames, hub.balance.Wallet())
		}

		if hub.desk != nil {
			out := make([]*broker.Position, 0)

			for position := range hub.desk.Positions() {
				out = append(out, position)
			}

			rows := make([]*wire.PositionT, 0, len(out))

			for _, position := range out {
				rows = append(rows, position.Wire())
			}

			frame := &wire.FrameT{
				Type: wire.FramePositionsFrame,
				Value: &wire.PositionsFrameT{
					Rows: rows,
				},
			}

			initialFrames = append(initialFrames, frame)
		}

		// The capture list rides the websocket with the rest of the
		// dashboard state: the dev-server origin cannot fetch the REST
		// route cross-origin, and the socket already reaches every client.
		if hub.captures != nil {
			frame := &wire.FrameT{
				Type: wire.FrameBacktestFrame,
				Value: &wire.BacktestFrameT{
					Captures: captureWire(hub.captures()),
				},
			}

			initialFrames = append(initialFrames, frame)
		}

		clientDone := make(chan struct{})
		received := make(chan struct{}, hub.writeWindow)
		clientCtx, cancelClient := context.WithCancel(hub.ctx)
		defer cancelClient()

		go func() {
			defer close(clientDone)
			defer cancelClient()

			for {
				select {
				case <-hub.ctx.Done():
					return
				default:
					messageType, payload, err := conn.Conn.ReadMessage()

					if err != nil {
						return
					}

					if messageType == websocket.BinaryMessage && len(payload) == 1 && payload[0] == 1 {
						select {
						case received <- struct{}{}:
						default:
						}
						continue
					}

					if messageType != websocket.TextMessage {
						continue
					}

					var request struct {
						Type      string `json:"type"`
						Symbol    string `json:"symbol"`
						At        string `json:"at"`
						CaptureID int64  `json:"captureId"`
					}

					if err := sonic.Unmarshal(payload, &request); err != nil {
						continue
					}

					switch request.Type {
					case "focus":
						types.SetFocus(request.Symbol)
					case "backtest.play":
						if hub.playback != nil {
							hub.playback.Play()
						}
					case "backtest.pause":
						if hub.playback != nil {
							hub.playback.Pause()
						}
					case "backtest.seek":
						if hub.playback != nil {
							if at, err := time.Parse(time.RFC3339Nano, request.At); err == nil {
								hub.playback.Seek(at)
							}
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
			}
		}()

		defer func() {
			_ = conn.Close()
			<-clientDone
		}()
		var deferredFrames []*types.UIFrame
		var inFlight uint64

		if len(initialFrames) > 0 {
			batch := telemetry.EncodeBatch(initialFrames)

			if len(batch.Bytes) > hub.maxMessageBytes {
				batch.Release()
				errnie.Error(errnie.Err(
					errnie.Validation,
					"dashboard: initial state exceeds websocket message limit",
					nil,
				))
				return
			}

			err := conn.Conn.WriteMessage(websocket.BinaryMessage, batch.Bytes)
			batch.Release()

			if err != nil {
				return
			}

			inFlight = 1
		}

		for {
			for inFlight >= hub.writeWindow {
				select {
				case <-hub.ctx.Done():
					return
				case <-clientDone:
					return
				case <-received:
					inFlight--
				case <-ui.Ready(consumerID):
					if ui.Overflowed(consumerID) {
						errnie.Error(errnie.Err(
							errnie.UnprocessableContent,
							"dashboard: client telemetry queue exceeded its configured limit",
							nil,
						))
						return
					}
				}
			}

			frames := deferredFrames
			deferredFrames = nil

			if len(frames) == 0 {
				frame, ok := ui.WaitPop(clientCtx, consumerID)

				if !ok {
					if ui.Overflowed(consumerID) {
						errnie.Error(errnie.Err(
							errnie.UnprocessableContent,
							"dashboard: client telemetry queue exceeded its configured limit",
							nil,
						))
					}

					return
				}

				frames = append(frames, frame)
				pending := ui.ConsumerLength(consumerID)

				for range pending {
					candidate, found := ui.Pop(consumerID)

					if !found {
						break
					}

					frames = append(frames, candidate)
				}
			}

			batchCount := len(frames)
			var batch *telemetry.BatchBuffer

			for {
				batch = telemetry.EncodeBatch(frames[:batchCount])

				if len(batch.Bytes) <= hub.maxMessageBytes {
					break
				}

				batch.Release()

				if batchCount == 1 {
					errnie.Error(errnie.Err(
						errnie.Validation,
						"dashboard: frame exceeds websocket message limit",
						nil,
					))
					return
				}

				batchCount = (batchCount + 1) / 2
			}

			err := conn.Conn.WriteMessage(websocket.BinaryMessage, batch.Bytes)
			batch.Release()

			if err != nil {
				if !expectedDashboardWriteClosure(err) {
					errnie.Error(errnie.Err(
						errnie.UnprocessableContent,
						"dashboard: write failed: "+err.Error(),
						err,
					))
				}

				return
			}

			inFlight++
			deferredFrames = frames[batchCount:]
		}
	}))

	hub.registerFluidWebRTC()

	return hub
}

/*
SetPlayback attaches the backtest driver and its capture listing so websocket
commands and the REST route reach it. Without a driver the controls are inert.
*/
func (hub *Hub) SetPlayback(
	controller interface {
		Play()
		Pause()
		Seek(at time.Time)
		Select(captureID int64)
		Hindsight(captureID int64)
	},
	captures func() []backtest.CaptureInfo,
) {
	// Upgrade-only: a session re-boot passes a nil controller with a fresh
	// capture list and must never displace the driving playback controller.
	if controller != nil {
		hub.playback = controller
	}

	if captures != nil {
		hub.captures = captures
	}
}

func captureWire(captures []backtest.CaptureInfo) []*wire.CaptureT {
	rows := make([]*wire.CaptureT, 0, len(captures))

	for _, capture := range captures {
		row := &wire.CaptureT{
			Id:        capture.ID,
			StartedAt: capture.StartedAt.UnixNano(),
			Frames:    capture.Frames,
		}

		if capture.EndedAt != nil {
			row.EndedAt = capture.EndedAt.UnixNano()
			row.HasEndedAt = true
		}

		rows = append(rows, row)
	}

	return rows
}

/*
expectedDashboardWriteClosure identifies transport errors that only report an
already completed dashboard disconnect.
*/
func expectedDashboardWriteClosure(err error) bool {
	for _, expected := range []error{
		syscall.EPIPE,
		syscall.ECONNRESET,
		io.EOF,
		io.ErrClosedPipe,
		fastwebsocket.ErrCloseSent,
	} {
		if errors.Is(err, expected) {
			return true
		}
	}

	return false
}

func (hub *Hub) Name() string { return "hub" }
func (hub *Hub) Error() error { return hub.err }

/*
Serve listens for dashboard websocket clients.
*/
func (hub *Hub) Run() error {
	if hub.err != nil {
		return hub.err
	}

	listener, err := net.Listen("tcp", hub.listenAddr)

	if err != nil {
		hub.err = err
		return hub.err
	}

	runDone := make(chan struct{})
	closeDone := make(chan error, 1)
	var fluidDone chan error

	if hub.fluid.Active() {
		fluidDone = make(chan error, 1)

		go func() {
			err := hub.fluid.Run()
			fluidDone <- err

			if err != nil {
				hub.cancel()
				_ = listener.Close()
			}
		}()
	}

	go func() {
		select {
		case <-hub.ctx.Done():
			closeDone <- listener.Close()
		case <-runDone:
			closeDone <- nil
		}
	}()

	appErr := hub.app.Listener(listener)
	hub.cancel()
	close(runDone)

	if errors.Is(appErr, net.ErrClosed) && hub.ctx.Err() != nil {
		appErr = nil
	}

	var fluidErr error

	if fluidDone != nil {
		fluidErr = <-fluidDone
	}

	listenerErr := <-closeDone

	if errors.Is(listenerErr, net.ErrClosed) {
		listenerErr = nil
	}

	hub.err = errors.Join(appErr, fluidErr, listenerErr)

	return hub.err
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
