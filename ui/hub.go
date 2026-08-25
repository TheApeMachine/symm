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

	"github.com/theapemachine/symm/nomagique/runtime"

	"github.com/bytedance/sonic"
	fastwebsocket "github.com/fasthttp/websocket"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/backtest"
	"github.com/theapemachine/symm/broker"
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
	ctx             context.Context
	cancel          context.CancelFunc
	err             error
	app             *fiber.App
	listenAddr      string
	bus             *runtime.Workspace
	thesis          *types.Thesis
	desk            *broker.Desk
	price           *broker.Price
	balance         *broker.Balance
	playback        playback
	captures        func() []backtest.CaptureInfo
	fluid           *FluidRTC
	diagnostics     DiagnosticsControl
	maxMessageBytes int
	maxBatchFrames  int
}

type diagnosticsToggleRequest struct {
	Enabled bool `json:"enabled"`
}

/*
DiagnosticsControl is the runtime switch the diagnostics page drives. It is a
narrow seam so the UI package never imports the trader package that owns the
collector.
*/
type DiagnosticsControl interface {
	SetDiagnosticsEnabled(enabled bool)
	DiagnosticsEnabled() bool
}

/*
SetDiagnosticsControl wires the runtime on/off switch into the dashboard hub.
*/
func (hub *Hub) SetDiagnosticsControl(control DiagnosticsControl) {
	hub.diagnostics = control
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
	bus *runtime.Workspace,
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
		thesis:     thesis,
		bus:        bus,
		desk:       desk,
		app: fiber.New(fiber.Config{
			JSONEncoder:     sonic.Marshal,
			JSONDecoder:     sonic.Unmarshal,
			StrictRouting:   true,
			ReadBufferSize:  4194304,
			WriteBufferSize: 4194304,
		}),
		price:   price,
		balance: balance,
		fluid: NewFluidRTC(ctx, runtime.ChannelOf[types.FluidFrame](
			bus, types.ChannelFluid,
			func(frame types.FluidFrame) string { return "" },
		), "fluid"),
		maxMessageBytes: viper.GetInt("ui.websocket.max_message_bytes"),
		maxBatchFrames:  viper.GetInt("ui.websocket.max_batch_frames"),
	}

	if hub.maxBatchFrames < 1 {
		hub.maxBatchFrames = 1
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

		if hub.bus == nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"dashboard: workspace bus is unavailable",
				nil,
			))
			return
		}

		ui := runtime.ChannelOf(
			hub.bus, types.ChannelUI,
			func(frame *types.UIFrame) string { return "" },
		)

		consumerID := consumerIDFor()

		// Each client subscribes its own bounded ring on the UI channel and
		// consumes it directly (like a signal consumes its inputs); no extra
		// frame channel hops between the ring and the socket. The workspace's
		// ring is bounded and drops oldest under overload, so a slow client
		// sheds frames instead of stalling the producer.
		ui.Subscribe(consumerID, func(frame *types.UIFrame) error {
			return writeUI(
				conn.Conn,
				hub.maxMessageBytes,
				[]*types.UIFrame{frame},
			)
		})
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

			if hub.desk.PositionStore != nil {
				recentTrades, err := hub.desk.PositionStore.RecentTrades(100)

				if err == nil && len(recentTrades) > 0 {
					openIdentities := make(map[string]struct{}, len(rows))

					for _, row := range rows {
						if row.Decision != nil && row.Decision.Id != "" {
							openIdentities[row.Decision.Id] = struct{}{}
						}
					}

					for _, trade := range recentTrades {
						if trade == nil || trade.Decision == nil {
							continue
						}

						if _, isOpen := openIdentities[trade.Decision.Id]; !isOpen {
							rows = append(rows, trade)
						}
					}
				}
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
		_, cancelClient := context.WithCancel(hub.ctx)
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
					case "position.exit":
						if hub.desk == nil {
							errnie.Error(errnie.Err(
								errnie.NotAcceptable,
								"dashboard: broker desk is unavailable for manual exit",
								nil,
							))
							continue
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
		}

		// Frames flow to the socket directly through the ChannelUI ring
		// subscription above; this handler only keeps the connection alive
		// until the client disconnects (the reader closes clientDone) or the
		// bus is torn down.
		select {
		case <-clientDone:
		case <-hub.ctx.Done():
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

/*
writeUI encodes one batch of UI frames and writes it to the dashboard socket,
splitting a batch that exceeds the per-message ceiling. It is the socket-side
half of the hub's direct ChannelUI consumption in the subscription step.
*/
func writeUI(conn *fastwebsocket.Conn, maxMessageBytes int, frames []*types.UIFrame) error {
	if len(frames) == 0 {
		return nil
	}

	batch := telemetry.EncodeBatch(frames)

	if len(batch.Bytes) <= maxMessageBytes {
		err := conn.WriteMessage(websocket.BinaryMessage, batch.Bytes)
		batch.Release()

		return err
	}

	batch.Release()

	if len(frames) == 1 {
		splitFrames, err := splitDashboardFrame(frames[0], maxMessageBytes)

		if err != nil {
			return err
		}

		return writeUI(conn, maxMessageBytes, splitFrames)
	}

	half := (len(frames) + 1) / 2

	if err := writeUI(conn, maxMessageBytes, frames[:half]); err != nil {
		return err
	}

	return writeUI(conn, maxMessageBytes, frames[half:])
}

/*
splitDashboardFrame divides a strategy update at decision boundaries until
every resulting FlatBuffers message fits the configured websocket limit.
*/
func splitDashboardFrame(
	frame *types.UIFrame,
	maxMessageBytes int,
) ([]*types.UIFrame, error) {
	batch := telemetry.EncodeBatch([]*types.UIFrame{frame})
	frameBytes := len(batch.Bytes)
	batch.Release()

	if frameBytes <= maxMessageBytes {
		return []*types.UIFrame{frame}, nil
	}

	strategy, valid := frame.Value.(*wire.StrategyFrameT)

	if frame.Type != wire.FrameStrategyFrame || !valid || len(strategy.Decisions) < 2 {
		return nil, fmt.Errorf(
			"dashboard: %v frame is %d bytes and exceeds websocket message limit %d",
			frame.Type,
			frameBytes,
			maxMessageBytes,
		)
	}

	middle := len(strategy.Decisions) / 2
	left, err := splitDashboardFrame(&wire.FrameT{
		Type: frame.Type,
		Value: &wire.StrategyFrameT{
			Evaluated: strategy.Evaluated,
			Outcome:   strategy.Outcome,
			Decisions: strategy.Decisions[:middle],
		},
	}, maxMessageBytes)

	if err != nil {
		return nil, err
	}

	right, err := splitDashboardFrame(&wire.FrameT{
		Type: frame.Type,
		Value: &wire.StrategyFrameT{
			Evaluated: strategy.Evaluated,
			Outcome:   strategy.Outcome,
			Decisions: strategy.Decisions[middle:],
		},
	}, maxMessageBytes)

	if err != nil {
		return nil, err
	}

	return append(left, right...), nil
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
