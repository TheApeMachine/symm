package public

import (
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
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/rawbus"
)

var socket *WebSocket
var socketOnce sync.Once

type WebSocketClient interface {
	Connect(EndpointType, string, uint64) error
	Tick() error
	Close() error
}

type WebSocket struct {
	ctx                context.Context
	cancel             context.CancelFunc
	err                error
	pool               *qpool.Q[any]
	conn               *websocket.Conn
	bus                *internal.Bus
	recorder           io.Writer
	streams            *sync.Map
	isConnected        atomic.Bool
	needsResubscribe   atomic.Bool
	wsPingInterval     time.Duration
	latencyProfilePath string
}

func NewWebSocket(
	ctx context.Context,
	pool *qpool.Q[any],
) *WebSocket {
	ctx, cancel := context.WithCancel(ctx)

	socketOnce.Do(func() {
		marketConfig, marketErr := config.LoadMarketConfig()
		wsPingInterval := time.Second

		if marketErr == nil {
			wsPingInterval = marketConfig.WSPingInterval
		}

		profilePath := viper.GetString("trading.paper.latency_profile")

		if profilePath == "" {
			profilePath = "runs/network_latency.json"
		}

		socket = &WebSocket{
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

var qvaluePool = sync.Pool{
	New: func() any {
		return &qpool.QValue[any]{}
	},
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
	ws.read()

	ticker := time.NewTicker(ws.wsPingInterval)
	defer ticker.Stop()

	for {
		for !ws.isConnected.Load() || ws.conn == nil {
			if ws.err = errnie.Error(ws.Connect(WebSocketURL, 0)); ws.err != nil {
				continue
			}

			if !ws.needsResubscribe.Load() {
				continue
			}

			if ws.err = errnie.Error(ws.resubscribe()); ws.err != nil {
				ws.disconnect()
				continue
			}

			ws.needsResubscribe.Store(false)
		}

		select {
		case <-ws.ctx.Done():
			return ws.err
		case <-ticker.C:
			if ws.conn == nil {
				ws.disconnect()
				continue
			}

			if errnie.Error(ws.conn.WriteJSON(PingMessage{
				Method: "ping",
				ReqID:  time.Now().UnixNano(),
			})) != nil {
				ws.disconnect()
				continue
			}
		default:
		}

		if !ws.isConnected.Load() || ws.conn == nil {
			continue
		}

		message := types.NewSocketMessage()

		if ws.err = errnie.Error(ws.conn.ReadJSON(message)); ws.err != nil {
			message.Release()
			ws.disconnect()
			continue
		}

		ws.dispatch(message)
		message.Release()
	}
}

func (ws *WebSocket) read() {
	go func() {
		for {
			slot := qvaluePool.Get().(*qpool.QValue[any])

			var message *qpool.QValue[any]

			if message, ws.err = ws.bus.Receive(
				"kraken:public",
			); errnie.Error(ws.err) != nil || message == nil {
				qvaluePool.Put(slot)
				break
			}

			qvaluePool.Put(slot)

			for ws.conn == nil || !ws.isConnected.Load() {
				if ws.ctx.Err() != nil {
					qvaluePool.Put(message)
					return
				}

				time.Sleep(10 * time.Millisecond)
			}

			if errnie.Error(ws.conn.WriteJSON(message.Value)) != nil {
				qvaluePool.Put(message)
				ws.disconnect()
				continue
			}

			qvaluePool.Put(message)
		}
	}()
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
