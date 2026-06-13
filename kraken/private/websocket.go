package private

import (
	"container/ring"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/kraken/wsutil"
	"github.com/theapemachine/symm/observability"
	"github.com/theapemachine/symm/rawbus"
)

const connectMaxAttempts = 12

type WebSocket struct {
	ctx            context.Context
	cancel         context.CancelFunc
	errMu           atomic.Pointer[error]
	pool            *qpool.Q[any]
	conn            *websocket.Conn
	rest            *Rest
	bus             *internal.Bus
	recorder        io.Writer
	streams         *sync.Map
	latencies       *ring.Ring
	isConnected     atomic.Bool
	bootstrapDone   atomic.Bool
	wsPingInterval  time.Duration
	tradingModel    string
	l3Enabled       bool
	readStarted     atomic.Bool
}

func NewWebSocket(
	ctx context.Context,
	pool *qpool.Q[any],
) *WebSocket {
	wsCtx, cancel := context.WithCancel(ctx)

	apiKey := os.Getenv("SYMM_KRAKEN_API_KEY")
	apiSecret := os.Getenv("SYMM_KRAKEN_API_SECRET")

	rest, err := NewRest(wsCtx, apiKey, apiSecret, public.EndpointWebSocketsToken)

	if err != nil {
		cancel()
		errnie.Error(err)
		return nil
	}

	types.BindTokenRest(rest)

	marketConfig, marketErr := config.LoadMarketConfig()

	if marketErr != nil {
		cancel()
		errnie.Error(marketErr)
		return nil
	}

	tradingConfig, tradingErr := config.LoadTradingConfig()

	if tradingErr != nil {
		cancel()
		errnie.Error(tradingErr)
		return nil
	}

	wsPingInterval := time.Second

	if marketConfig.WSPingInterval > 0 {
		wsPingInterval = marketConfig.WSPingInterval
	}

	socket := &WebSocket{
		ctx:            wsCtx,
		cancel:         cancel,
		pool:           pool,
		rest:           rest,
		wsPingInterval: wsPingInterval,
		tradingModel:   tradingConfig.Model,
		l3Enabled:      marketConfig.L3Enabled,
		bus: internal.NewBus(
			wsCtx,
			pool,
			[]internal.Channel{internal.ChannelRaw, internal.ChannelLevel3, internal.ChannelKrakenPublic, internal.ChannelUI},
			[]internal.Subscription{
				internal.Subscribe(internal.ChannelRaw, "kraken:private:raw"),
				internal.Subscribe(internal.ChannelLevel3, "kraken:private:level3"),
				internal.Subscribe(internal.ChannelKrakenPublic, "kraken:private:public"),
				internal.Subscribe(internal.ChannelKrakenPrivate, "kraken:private:bus"),
			},
		),
		streams:   &sync.Map{},
		latencies: ring.New(64),
	}

	errnie.Info("kraken/private websocket ready")

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
	endpoint public.EndpointType, attempt uint64,
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
			string(endpoint), http.Header{},
		)

		if connectErr == nil {
			ws.setErr(nil)
			ws.isConnected.Store(true)
			observability.Shared().RecordWebSocketConnected(
				"kraken/private",
				string(endpoint),
				time.Now().UTC(),
			)
			return nil
		}

		ws.setErr(connectErr)
		lastErr = connectErr
		observability.Shared().RecordWebSocketReconnect(
			"kraken/private",
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

	connectErr := errors.New("kraken/private websocket: connect failed after max attempts")

	if lastErr != nil {
		connectErr = lastErr
	}

	ws.setErr(connectErr)

	return connectErr
}

func (ws *WebSocket) disconnect() {
	ws.isConnected.Store(false)

	if ws.conn == nil {
		return
	}

	errnie.Error(ws.conn.Close())
	ws.conn = nil
}

func (ws *WebSocket) Tick() (err error) {
	if ws.readStarted.CompareAndSwap(false, true) {
		go ws.readLoop()
	}

	ticker := time.NewTicker(ws.wsPingInterval)
	defer ticker.Stop()

	for {
		paperLevel3 := ws.tradingModel == "paper" && ws.l3Enabled

		if paperLevel3 || ws.tradingModel != "paper" {
			endpoint := public.WebSocketAuthURL

			if paperLevel3 {
				endpoint = public.WebSocketL3URL
			}

			if connectErr := errnie.Error(ws.Connect(endpoint, 0)); connectErr != nil {
				ws.setErr(connectErr)
				continue
			}

			if ws.bootstrapDone.CompareAndSwap(false, true) {
				if err := ws.subscribeBalances(); errnie.Error(err) != nil {
					errnie.Error(err)
				}
			}
		} else {
			select {
			case <-ws.ctx.Done():
				return ws.ctx.Err()
			default:
				time.Sleep(ws.wsPingInterval)
			}

			continue
		}

		select {
		case <-ws.ctx.Done():
			return ws.getErr()
		case <-ticker.C:
			if errnie.Error(ws.conn.WriteJSON(public.PingMessage{
				Method: "ping",
				ReqID:  time.Now().UnixNano(),
			})) != nil {
				ws.disconnect()
			}
		}

		if !ws.isConnected.Load() || ws.conn == nil {
			continue
		}

		message := types.NewSocketMessage()

		readWait := ws.wsPingInterval

		if deadlineErr := ws.conn.SetReadDeadline(time.Now().Add(readWait)); deadlineErr != nil {
			ws.setErr(deadlineErr)
			message.Release()
			errnie.Error(deadlineErr)
			ws.disconnect()
			continue
		}

		if readErr := ws.conn.ReadJSON(message); readErr != nil {
			ws.setErr(readErr)
			message.Release()

			var netErr net.Error

			if !errors.As(readErr, &netErr) || !netErr.Timeout() {
				errnie.Error(readErr)
			}

			ws.disconnect()
			continue
		}

		if len(message.Errors) > 0 || (message.Success != nil && !*message.Success) {
			ws.handleErrors(message)
			message.Release()
			continue
		}

		switch message.Channel {
		case "pong":
			pong := public.PongMessage{}

			if err := errnie.Error(message.Unmarshal(&pong)); err != nil {
				message.Release()
				continue
			}

			ws.latencies.Value = time.Since(pong.TimeIn)
			ws.latencies.Next()
		case "heartbeat":
			ws.isConnected.Store(true)
		case "balances":
			balances := user.Balances{}

			if err := errnie.Error(message.Unmarshal(&balances)); err != nil {
				message.Release()
				continue
			}

			rawbus.Send(ws.bus, rawbus.TypeBalances, balances)
			ws.bus.Send(internal.ChannelUI, "balances", balances)
		case "orders":
			orders := []trading.OrderUpdate{}

			if err := errnie.Error(message.Unmarshal(&orders)); err != nil {
				message.Release()
				continue
			}

			rawbus.Send(ws.bus, rawbus.TypeOrders, orders)
		case "executions":
			executions := []user.Execution{}

			if err := errnie.Error(message.Unmarshal(&executions)); err != nil {
				message.Release()
				continue
			}

			rawbus.Send(ws.bus, rawbus.TypeExecutions, executions)
		case "level3":
			level3Updates := make([]market.Level3Update, 0)

			if err := errnie.Error(message.Unmarshal(&level3Updates)); err != nil || len(level3Updates) == 0 {
				message.Release()
				continue
			}

			for index := range level3Updates {
				update := level3Updates[index]
				rawbus.Send(ws.bus, rawbus.TypeLevel3, &update)
			}
		}

		message.Release()
	}
}

func (ws *WebSocket) readLoop() {
	for {
		message, receiveErr := ws.bus.Receive(internal.ChannelKrakenPrivate)

		if errnie.Error(receiveErr) != nil || message == nil {
			break
		}

		for ws.conn == nil || !ws.isConnected.Load() {
			if ws.ctx.Err() != nil {
				return
			}

			time.Sleep(10 * time.Millisecond)
		}

		if ws.tradingModel == "paper" {
			switch message.Type {
			case public.Level3Channel, "unsubscribe":
			default:
				continue
			}
		}

		frame, ok := message.Value.(types.KrakenMessage)

		if !ok {
			continue
		}

		if errnie.Error(ws.conn.WriteJSON(frame)) != nil {
			ws.disconnect()
			continue
		}
	}
}
