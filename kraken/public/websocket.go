package public

import (
	"container/ring"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
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
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
)

var socket *WebSocket
var socketOnce sync.Once

type WebSocketClient interface {
	Connect(EndpointType, string, uint64) error
	Tick() error
	Close() error
}

type WebSocket struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	conn        *websocket.Conn
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
	ctx, cancel := context.WithCancel(ctx)

	socketOnce.Do(func() {
		socket = &WebSocket{
			ctx:    ctx,
			cancel: cancel,
			pool:   pool,
			bus: internal.NewBus(
				ctx,
				pool,
				[]string{"raw", "level3", "kraken:public", "ui"},
				[]string{"raw", "level3", "kraken:public"},
			),
			streams:   &sync.Map{},
			latencies: ring.New(64),
		}

		errnie.Info("kraken/public websocket ready")
	})

	return socket
}

func (ws *WebSocket) Connect(
	endpoint EndpointType, n uint64,
) error {
	if ws.isConnected.Load() && ws.conn != nil {
		return nil
	}

	ws.isConnected.Store(false)

	var response *http.Response

	if ws.conn, response, ws.err = websocket.DefaultDialer.Dial(
		string(endpoint), nil,
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
		for !ws.isConnected.Load() || ws.conn == nil {
			if ws.err = errnie.Error(ws.Connect(WebSocketURL, 0)); ws.err != nil {
				continue
			}
		}

		select {
		case <-ws.ctx.Done():
			return ws.err
		case <-ticker.C:
			if ws.conn == nil {
				ws.disconnect()
				break
			}

			if errnie.Error(ws.conn.WriteJSON(PingMessage{
				Method: "ping",
				ReqID:  time.Now().UnixNano(),
			})) != nil {
				ws.disconnect()
			}
		default:
			slot := qvaluePool.Get().(*qpool.QValue[any])

			var message *qpool.QValue[any]

			if message, ws.err = ws.bus.Poll(
				"kraken:public",
			); errnie.Error(ws.err) != nil || message == nil {
				qvaluePool.Put(slot)
				break
			}

			qvaluePool.Put(slot)

			if errnie.Error(ws.conn.WriteJSON(message.Value)) != nil {
				qvaluePool.Put(message)
				ws.disconnect()
				continue
			}

			qvaluePool.Put(message)
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

		var object types.Unmarshaler

		switch message.Channel {
		case "pong":
			pong := PongMessage{}

			if err := errnie.Error(message.Unmarshal(&pong)); err != nil {
				message.Release()
				continue
			}

			ws.latencies.Value = time.Since(pong.TimeIn)
			ws.latencies.Next()
			ws.recordLatency()
		case "heartbeat":
			ws.isConnected.Store(true)
		case "ohlc":
			object = &market.CandleUpdate{}
		case "instrument":
			object = &market.InstrumentUpdate{}
		case "ticker":
			object = &market.TickerUpdate{}
		case "book":
			object = &market.BookUpdate{}
		case "trade":
			object = &market.TradeUpdates{}
		case "execution":
			object = &user.Execution{}
		}

		if err := errnie.Error(object.Unmarshal(message)); err != nil {
			message.Release()
			continue
		}

		ws.bus.Send("raw", message.Channel, object)
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
	latencyFile, err := os.OpenFile(filepath.Join(
		"runs", "network_latency.json"),
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		0644,
	)

	if errnie.Error(err) != nil {
		return
	}

	ws.latencies.Do(func(value any) {
		duration, _ := value.(time.Duration)
		fmt.Fprintf(latencyFile, "%d\n", duration)
	})

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
