package private

import (
	"container/ring"
	"context"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/rawbus"
)

var socket *WebSocket
var socketOnce sync.Once

type WebSocket struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	conn        *websocket.Conn
	rest        *Rest
	bus         *internal.Bus
	recorder    io.Writer
	streams     *sync.Map
	latencies   *ring.Ring
	isConnected atomic.Bool
}

func NewWebSocket(
	ctx context.Context,
	pool *qpool.Q[any],
) *WebSocket {
	var initErr error

	socketOnce.Do(func() {
		wsCtx, cancel := context.WithCancel(ctx)

		apiKey := os.Getenv("SYMM_KRAKEN_API_KEY")
		apiSecret := os.Getenv("SYMM_KRAKEN_API_SECRET")

		rest, err := NewRest(wsCtx, apiKey, apiSecret, public.EndpointWebSocketsToken)

		if err != nil {
			cancel()
			initErr = err
			return
		}

		types.BindTokenRest(rest)

		socket = &WebSocket{
			ctx:    wsCtx,
			cancel: cancel,
			pool:   pool,
			rest:   rest,
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
	})

	if initErr != nil {
		errnie.Error(initErr)
		return nil
	}

	return socket
}

func (ws *WebSocket) Connect(
	endpoint public.EndpointType, n uint64,
) error {
	if ws.isConnected.Load() && ws.conn != nil {
		return nil
	}

	ws.isConnected.Store(false)

	var response *http.Response

	if ws.conn, response, ws.err = websocket.DefaultDialer.Dial(
		string(endpoint), http.Header{},
	); ws.err != nil {
		if response != nil {
			errnie.Error(ws.err, response.StatusCode, response.Status)
		} else {
			errnie.Error(ws.err)
		}

		// Fibonacci gives us a good exponential backoff.
		n = uint64(
			math.Round((math.Pow(
				math.Phi, float64(n),
			) + math.Pow(
				math.Phi-1, float64(n),
			)) / math.Sqrt(5)),
		)

		time.Sleep(time.Duration(n) * time.Second)
		return ws.Connect(endpoint, n)
	}

	ws.isConnected.Store(true)
	return nil
}

func (ws *WebSocket) disconnect() {
	ws.isConnected.Store(false)

	if ws.conn == nil {
		return
	}

	errnie.Error(ws.conn.Close())
	ws.conn = nil
}

var qvaluePool = sync.Pool{
	New: func() any {
		return &qpool.QValue[any]{}
	},
}

func (ws *WebSocket) Tick() (err error) {
	ticker := time.NewTicker(
		viper.GetDuration("market.ws_ping_interval"),
	)
	defer ticker.Stop()

	for {
		paperLevel3 := viper.GetString("trading.model") == "paper" && viper.GetBool("market.l3_enabled")

		if paperLevel3 || viper.GetString("trading.model") != "paper" {
			endpoint := public.WebSocketAuthURL

			if paperLevel3 {
				endpoint = public.WebSocketL3URL
			}

			if ws.err = errnie.Error(ws.Connect(endpoint, 0)); ws.err != nil {
				continue
			}
		} else {
			select {
			case <-ws.ctx.Done():
				return ws.ctx.Err()
			default:
				time.Sleep(viper.GetDuration("market.ws_ping_interval"))
			}

			continue
		}

		select {
		case <-ws.ctx.Done():
			return ws.err
		case <-ticker.C:
			if errnie.Error(ws.conn.WriteJSON(public.PingMessage{
				Method: "ping",
				ReqID:  time.Now().UnixNano(),
			})) != nil {
				ws.disconnect()
			}
		default:
			slot := qvaluePool.Get().(*qpool.QValue[any])

			var message *qpool.QValue[any]

			if message, ws.err = ws.bus.Poll(
				"kraken:private",
			); errnie.Error(ws.err) != nil || message == nil {
				qvaluePool.Put(slot)
				break
			}

			qvaluePool.Put(slot)

			if viper.GetString("trading.model") == "paper" {
				switch message.Type {
				case public.Level3Channel, "unsubscribe":
				default:
					qvaluePool.Put(message)
					continue
				}
			}

			frame, ok := message.Value.(types.KrakenMessage)

			if !ok {
				qvaluePool.Put(message)
				continue
			}

			if errnie.Error(ws.conn.WriteJSON(frame)) != nil {
				qvaluePool.Put(message)
				ws.disconnect()
				continue
			}

			qvaluePool.Put(message)
		}

		if !ws.isConnected.Load() || ws.conn == nil {
			continue
		}

		message := types.NewSocketMessage()

		readWait := viper.GetDuration("market.ws_ping_interval")

		if ws.err = ws.conn.SetReadDeadline(time.Now().Add(readWait)); ws.err != nil {
			message.Release()
			errnie.Error(ws.err)
			ws.disconnect()
			continue
		}

		if ws.err = ws.conn.ReadJSON(message); ws.err != nil {
			message.Release()

			var netErr net.Error

			if !errors.As(ws.err, &netErr) || !netErr.Timeout() {
				errnie.Error(ws.err)
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

func (ws *WebSocket) handleErrors(message *types.SocketMessage) {
	for _, err := range message.Errors {
		switch strings.Split(err, ":")[0] {
		case "EOrder":
			time.Sleep(1 * time.Second)
		case "EService":
			unixTimestamp, err := strconv.ParseInt(strings.Split(err, ":")[1], 10, 64)

			if errnie.Error(err) != nil {
				continue
			}

			time.Sleep(time.Until(time.Unix(unixTimestamp, 0)))
		default:
			errnie.Error(errors.New(err))
		}
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

func addParams(value any) (trading.AddParams, bool) {
	switch typed := value.(type) {
	case trading.AddParams:
		return typed, true
	case *trading.AddParams:
		if typed == nil {
			return trading.AddParams{}, false
		}

		return *typed, true
	default:
		return trading.AddParams{}, false
	}
}
