package public

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/kraken/wsutil"
	"github.com/theapemachine/symm/observability"
	"github.com/theapemachine/symm/rawbus"
)

const connectMaxAttempts = 12

type WebSocketClient interface {
	Connect(EndpointType, string, uint64) error
	Tick() error
	Close() error
}

type WebSocket struct {
	ctx                context.Context
	cancel             context.CancelFunc
	errMu              atomic.Pointer[error]
	pool               *qpool.Q[any]
	conn               *websocket.Conn
	bus                *internal.Bus
	recorder           io.Writer
	streams            *sync.Map
	isConnected        atomic.Bool
	needsResubscribe   atomic.Bool
	wsPingInterval     time.Duration
	latencyProfilePath string
	readStarted        atomic.Bool
}

func NewWebSocket(
	ctx context.Context,
	pool *qpool.Q[any],
) *WebSocket {
	ctx, cancel := context.WithCancel(ctx)
	marketConfig, marketErr := config.LoadMarketConfig()
	wsPingInterval := time.Second

	if marketErr == nil && marketConfig.WSPingInterval > 0 {
		wsPingInterval = marketConfig.WSPingInterval
	}

	profilePath := viper.GetString("trading.paper.latency_profile")

	if profilePath == "" {
		profilePath = "runs/network_latency.json"
	}

	socket := &WebSocket{
		ctx:                ctx,
		cancel:             cancel,
		pool:               pool,
		wsPingInterval:     wsPingInterval,
		latencyProfilePath: profilePath,
		bus: internal.NewBus(
			ctx,
			pool,
			[]internal.Channel{internal.ChannelRaw, internal.ChannelLevel3, internal.ChannelKrakenPublic, internal.ChannelUI},
			[]internal.Subscription{
				internal.Subscribe(internal.ChannelRaw, "kraken:public:raw"),
				internal.Subscribe(internal.ChannelLevel3, "kraken:public:level3"),
				internal.Subscribe(internal.ChannelKrakenPublic, "kraken:public:bus"),
			},
		),
		streams: &sync.Map{},
	}

	errnie.Info("kraken/public websocket ready")

	return socket
}

func (ws *WebSocket) setErr(err error) {
	if err == nil {
		ws.errMu.Store(nil)
		return
	}

	stored := err
	ws.errMu.Store(&stored)
}

func (ws *WebSocket) getErr() error {
	err := ws.errMu.Load()

	if err == nil {
		return nil
	}

	return *err
}

func (ws *WebSocket) Connect(
	endpoint EndpointType, attempt uint64,
) error {
	if ws.isConnected.Load() && ws.conn != nil {
		return nil
	}

	connectCtx := wsutil.NonNilContext(ws.ctx)
	backoff := wsutil.NewBackoffFromConfig()
	var lastErr error

	for connectAttempt := attempt; connectAttempt < attempt+connectMaxAttempts; connectAttempt++ {
		if connectCtx.Err() != nil {
			ws.setErr(connectCtx.Err())
			return connectCtx.Err()
		}

		ws.isConnected.Store(false)

		var response *http.Response
		var connectErr error

		ws.conn, response, connectErr = websocket.DefaultDialer.Dial(
			string(endpoint), nil,
		)

		if connectErr == nil {
			ws.setErr(nil)
			ws.isConnected.Store(true)
			observability.Shared().RecordWebSocketConnected(
				"kraken/public",
				string(endpoint),
				time.Now().UTC(),
			)
			return nil
		}

		ws.setErr(connectErr)
		lastErr = connectErr
		observability.Shared().RecordWebSocketReconnect(
			"kraken/public",
			string(endpoint),
			connectErr.Error(),
			time.Now().UTC(),
		)

		if response != nil {
			errnie.Error(connectErr, response.StatusCode, response.Status)
		} else {
			errnie.Error(connectErr)
		}

		if waitErr := backoff.Wait(connectCtx, connectAttempt); waitErr != nil {
			ws.setErr(waitErr)
			return waitErr
		}
	}

	connectErr := fmt.Errorf(
		"kraken/public websocket: connect failed after %d attempts",
		connectMaxAttempts,
	)

	if lastErr != nil {
		connectErr = lastErr
	}

	ws.setErr(connectErr)

	return connectErr
}

func (ws *WebSocket) disconnect() {
	ws.isConnected.Store(false)
	ws.needsResubscribe.Store(true)

	if ws.conn == nil {
		return
	}

	errnie.Error(ws.conn.Close())
	ws.conn = nil
}

func (ws *WebSocket) resubscribe() error {
	if err := rawbus.Send(ws.bus, rawbus.TypeReconnect, struct{}{}); err != nil {
		return err
	}

	return ws.bus.Send(
		internal.ChannelKrakenPublic,
		"instrument",
		types.KrakenMessage{
			Method: "subscribe",
			Params: market.InstrumentParams{
				Channel:  "instrument",
				Snapshot: true,
			},
			ReqID: time.Now().UnixNano(),
		},
	)
}

func (ws *WebSocket) dispatch(message *types.SocketMessage) {
	if len(message.Errors) > 0 || (message.Success != nil && !*message.Success) {
		ws.handleErrors(message)
		return
	}

	if message.Success != nil && *message.Success {
		return
	}

	var object types.Unmarshaler

	switch message.Channel {
	case "pong":
		ws.recordLatency(message)
		return
	case "heartbeat":
		ws.isConnected.Store(true)
		return
	case "ohlc":
		object = &market.CandleUpdates{}
	case "instrument":
		object = &market.InstrumentUpdate{}
	case "ticker":
		object = &market.TickerUpdates{}
	case "book":
		object = &market.BookUpdates{}
	case "trade":
		object = &market.TradeUpdates{}
	case "execution":
		object = &user.Execution{}
	case "status":
		return
	default:
		if message.Channel != "" {
			errnie.Debug(fmt.Sprintf("kraken/public: ignored channel %q", message.Channel))
		}

		return
	}

	if err := errnie.Error(object.Unmarshal(message)); err != nil {
		return
	}

	rawbus.Send(ws.bus, rawbus.Type(message.Channel), object)
}

func (ws *WebSocket) Tick() (err error) {
	if ws.readStarted.CompareAndSwap(false, true) {
		go ws.readLoop()
	}

	for {
		for !ws.isConnected.Load() || ws.conn == nil {
			if connectErr := errnie.Error(ws.Connect(WebSocketURL, 0)); connectErr != nil {
				ws.setErr(connectErr)
				continue
			}

			if !ws.needsResubscribe.Load() {
				continue
			}

			if resubErr := errnie.Error(ws.resubscribe()); resubErr != nil {
				ws.setErr(resubErr)
				ws.disconnect()
				continue
			}

			ws.needsResubscribe.Store(false)
		}

		select {
		case <-ws.ctx.Done():
			return ws.getErr()
		default:
		}

		if !ws.isConnected.Load() || ws.conn == nil {
			continue
		}

		message := types.NewSocketMessage()

		if readErr := errnie.Error(ws.conn.ReadJSON(message)); readErr != nil {
			ws.setErr(readErr)
			message.Release()
			ws.disconnect()
			continue
		}

		ws.dispatch(message)
		message.Release()
	}
}

func (ws *WebSocket) readLoop() {
	for {
		var message *qpool.QValue[any]

		receiveErr := error(nil)

		message, receiveErr = ws.bus.Receive("kraken:public")

		if errnie.Error(receiveErr) != nil || message == nil {
			break
		}

		for ws.conn == nil || !ws.isConnected.Load() {
			if ws.ctx.Err() != nil {
				return
			}

			time.Sleep(10 * time.Millisecond)
		}

		if errnie.Error(ws.conn.WriteJSON(message.Value)) != nil {
			ws.disconnect()
			continue
		}
	}
}

func (ws *WebSocket) recordLatency(message *types.SocketMessage) {
	pong := PongMessage{}

	if err := errnie.Error(message.Unmarshal(&pong)); err != nil {
		return
	}

	if pong.TimeIn.IsZero() || pong.TimeOut.IsZero() {
		return
	}

	serverLatency := pong.TimeOut.Sub(pong.TimeIn)
	inboundLatency := time.Since(pong.TimeOut)

	if inboundLatency < 0 {
		return
	}

	roundTrip := inboundLatency + serverLatency + inboundLatency

	if roundTrip <= 0 {
		return
	}

	profilePath := ws.latencyProfilePath

	if profilePath == "" {
		profilePath = "runs/network_latency.json"
	}

	if errnie.Error(os.MkdirAll(filepath.Dir(profilePath), 0o755)) != nil {
		return
	}

	latencyFile, err := os.OpenFile(
		profilePath,
		os.O_CREATE|os.O_APPEND|os.O_WRONLY,
		0o644,
	)

	if errnie.Error(err) != nil {
		return
	}

	fmt.Fprintf(latencyFile, "%d\n", roundTrip.Nanoseconds())

	if errnie.Error(latencyFile.Close()) != nil {
		return
	}
}

func (ws *WebSocket) Close() error {
	if ws.conn != nil {
		errnie.Error(ws.conn.Close())
	}

	ws.isConnected.Store(false)
	ws.cancel()

	return nil
}
