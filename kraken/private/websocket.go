package private

import (
	"container/ring"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
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
)

var socket *WebSocket
var socketOnce sync.Once

type WebSocket struct {
	ctx           context.Context
	cancel        context.CancelFunc
	err           error
	pool          *qpool.Q[any]
	conn          *websocket.Conn
	tokenProvider *TokenProvider
	bus           *internal.Bus
	recorder      io.Writer
	streams       *sync.Map
	latencies     *ring.Ring
	isConnected   atomic.Bool
}

func NewWebSocket(
	ctx context.Context,
	pool *qpool.Q[any],
) *WebSocket {
	ctx, cancel := context.WithCancel(ctx)

	tokenProvider, err := NewTokenProvider(
		ctx,
		os.Getenv("SYMM_KRAKEN_API_KEY"),
		os.Getenv("SYMM_KRAKEN_API_SECRET"),
	)

	if err != nil {
		errnie.Error(err)
		return nil
	}

	socketOnce.Do(func() {
		socket = &WebSocket{
			ctx:           ctx,
			cancel:        cancel,
			pool:          pool,
			tokenProvider: tokenProvider,
			bus: internal.NewBus(
				ctx,
				pool,
				[]string{"raw", "level3", "kraken:public", "ui"},
				[]string{"raw", "level3", "kraken:public", "kraken:private"},
			),
			streams:   &sync.Map{},
			latencies: ring.New(64),
		}

		errnie.Info("kraken/public websocket ready")
	})

	return socket
}

func (ws *WebSocket) Connect(
	endpoint public.EndpointType, n uint64,
) error {
	if ws.isConnected.Load() {
		return nil
	}

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
		if ws.err = errnie.Error(ws.Connect(public.WebSocketAuthURL, 0)); ws.err != nil {
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
				qvaluePool.Put(message)
				continue
			}

			frame, ok := message.Value.(types.KrakenMessage)

			if !ok {
				qvaluePool.Put(message)
				continue
			}

			token, err := ws.tokenProvider.Token(ws.ctx)

			if errnie.Error(err) != nil {
				qvaluePool.Put(message)
				continue
			}

			outbound, frameErr := frameWithToken(frame, token)

			if errnie.Error(frameErr) != nil {
				qvaluePool.Put(message)
				continue
			}

			if errnie.Error(ws.conn.WriteJSON(outbound)) != nil {
				qvaluePool.Put(message)
				ws.disconnect()
				continue
			}

			qvaluePool.Put(message)
		}

		if !ws.isConnected.Load() {
			continue
		}

		message := types.NewSocketMessage()

		if ws.err = errnie.Error(ws.conn.ReadJSON(message)); ws.err != nil {
			message.Release()
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

			ws.bus.Send("raw", "balances", balances)
			ws.bus.Send("ui", "balances", balances)
		case "orders":
			orders := []trading.OrderUpdate{}

			if err := errnie.Error(message.Unmarshal(&orders)); err != nil {
				message.Release()
				continue
			}

			ws.bus.Send("raw", "orders", orders)
		case "executions":
			executions := []user.Execution{}

			if err := errnie.Error(message.Unmarshal(&executions)); err != nil {
				message.Release()
				continue
			}

			ws.bus.Send("raw", "executions", executions)
		case "level3":
			level3 := market.Level3Update{}

			if err := errnie.Error(message.Unmarshal(&level3)); err != nil {
				message.Release()
				continue
			}

			ws.bus.Send("raw", "level3", level3)
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

func (ws *WebSocket) recordLatency() {
	// Atomic replace (temp + rename) with truncation: the previous O_WRONLY
	// overwrite-in-place could leave stale trailing bytes, and the file was only
	// ever written on a 1-in-64 coin flip of the wall clock.
	path := "runs/network_latency.json"
	tempPath := path + ".tmp"

	latencyFile, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)

	if err != nil {
		errnie.Error(err)
		return
	}

	ws.latencies.Do(func(value any) {
		duration, _ := value.(time.Duration)
		fmt.Fprintf(latencyFile, "%d\n", duration)
	})

	if err := latencyFile.Close(); err != nil {
		errnie.Error(err)
		return
	}

	if err := os.Rename(tempPath, path); err != nil {
		errnie.Error(err)
	}
}

func (ws *WebSocket) publishOhlc(message *types.SocketMessage) {
	var candles []market.CandleUpdate

	if err := errnie.Error(
		message.Unmarshal(&candles),
	); err != nil {
		return
	}

	for _, candle := range candles {
		ws.streams.Range(func(key, value any) bool {
			if key == candle.Symbol {
				ws.bus.Send("ui", "ohlc", candle)
			}

			return true
		})
	}
}

func frameWithToken(frame types.KrakenMessage, token string) (types.KrakenMessage, error) {
	var params map[string]any

	if err := sonic.Unmarshal(frame.Params, &params); err != nil {
		return types.KrakenMessage{}, err
	}

	params["token"] = token

	raw, err := sonic.Marshal(params)

	if err != nil {
		return types.KrakenMessage{}, err
	}

	return types.KrakenMessage{
		Method: frame.Method,
		Params: raw,
		ReqID:  frame.ReqID,
	}, nil
}

func (ws *WebSocket) Close() error {
	if ws.conn != nil {
		errnie.Error(ws.conn.Close())
	}

	ws.isConnected.Store(false)
	ws.cancel()

	return nil
}
