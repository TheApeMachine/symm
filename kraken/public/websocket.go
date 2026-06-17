package public

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/types"
)

/*
WebSocket is the Kraken public websocket client.
*/
type WebSocket struct {
	ctx             context.Context
	cancel          context.CancelFunc
	err             error
	pool            *qpool.Q[any]
	broadcasts      *sync.Map
	subscribers     *sync.Map
	conn            *websocket.Conn
	isConnected     atomic.Bool
	connectMaxDelay int
}

/*
NewWebSocket creates a new Kraken public websocket client.
*/
func NewWebSocket(
	ctx context.Context, pool *qpool.Q[any],
) *WebSocket {
	ctx, cancel := context.WithCancel(ctx)

	socket := &WebSocket{
		ctx:             ctx,
		cancel:          cancel,
		pool:            pool,
		broadcasts:      &sync.Map{},
		subscribers:     &sync.Map{},
		isConnected:     atomic.Bool{},
		connectMaxDelay: viper.GetInt("system.network.connection.max_delay"),
	}

	for _, channel := range []string{
		"ohlc", "instrument", "ticker", "book", "trade", "execution", "status",
	} {
		socket.broadcasts.Store(
			channel, socket.pool.CreateBroadcastGroup(channel),
		)
	}

	for _, channel := range []string{"kraken:public"} {
		socket.subscribers.Store(
			channel, pool.Subscribe(channel, socket.onMessage),
		)
	}

	errnie.Info("kraken/public: websocket client ready")
	return socket
}

/*
onMessage will be called by the qpool.BroadcastGroup for every consumer
that has subscribed with a callback function.
*/
func (ws *WebSocket) onMessage(artifact *datura.Artifact) error {
	destination := errnie.Does(func() (string, error) {
		return artifact.Destination()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken/public: failed to get destination",
			err,
		))
	}).Value()

	switch destination {
	case "kraken:public":
		payload := errnie.Does(func() ([]byte, error) {
			return artifact.DecryptPayload()
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken/public: failed to get payload",
				err,
			))
		}).Value()

		ws.writeOutbound(payload)
	default:
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken/public: ignored destination",
			errors.New(destination),
		))
	}

	return nil
}

/*
Run reads Kraken public websocket frames and routes them into broadcast groups.
*/
func (ws *WebSocket) Run(endpoint EndpointType) {
	errnie.Info("kraken/public: websocket client running")

	for {
		select {
		case <-ws.ctx.Done():
			return
		default:
		}

		if !ws.isConnected.Load() || ws.conn == nil {
			if err := ws.Connect(endpoint, 1); err != nil {
				select {
				case <-ws.ctx.Done():
					return
				case <-time.After(time.Second):
				}

				continue
			}
		}

		message := types.Acquire()

		var payload []byte

		_, payload, ws.err = ws.conn.ReadMessage()

		if ws.err != nil {
			message.Release()
			ws.dropConnection()

			continue
		}

		if ws.err = errnie.Error(
			message.Decode(payload),
		); ws.err != nil {
			message.Release()

			continue
		}

		ws.routeInbound(message)
		message.Release()
	}
}

func (ws *WebSocket) routeInbound(message *types.SocketMessage) {
	if message == nil || len(message.Data) == 0 {
		return
	}

	artifact := datura.Acquire("kraken:public", datura.Artifact_Type_json)

	if artifact == nil {
		return
	}

	artifact.WithRole(
		message.Channel,
	).WithScope(
		message.Type,
	).WithDestination(
		message.Channel,
	)

	if artifact.WithPayload(message.Data) == nil {
		return
	}

	artifact.Poke(
		"success", strconv.FormatBool(message.Success),
	).Poke(
		"time_in", message.TimeIn.Format(time.RFC3339),
	).Poke(
		"time_out", message.TimeOut.Format(time.RFC3339),
	)

	if message.Error != "" {
		artifact.WithError(errnie.Err(
			errnie.Unknown,
			"kraken/public: error",
			errors.New(message.Error),
		))
	}

	krakenmarket.InsertMarketArtifact(krakenmarket.MarketTree(), artifact)

	if bg, ok := ws.broadcasts.Load(datura.Peek[string](artifact, "role")); ok {
		errnie.Error(bg.(*qpool.BroadcastGroup).Send(artifact))
	}
}

func (ws *WebSocket) writeOutbound(payload []byte) {
	if len(payload) == 0 {
		return
	}

	if !ws.isConnected.Load() || ws.conn == nil {
		return
	}

	errnie.Error(ws.conn.WriteMessage(websocket.TextMessage, payload))
}

func (ws *WebSocket) publishStatus(scope string) {
	artifact := datura.Acquire(
		"kraken:public", datura.Artifact_Type_json,
	).WithDestination(
		"kraken:public",
	).WithRole(
		"status",
	).WithScope(
		scope,
	)

	if bg, ok := ws.broadcasts.Load("status"); ok {
		errnie.Error(bg.(*qpool.BroadcastGroup).Send(artifact))
	}
}

func (ws *WebSocket) dropConnection() {
	ws.isConnected.Store(false)

	if ws.conn == nil {
		ws.publishStatus("disconnected")

		return
	}

	errnie.Error(ws.conn.Close())
	ws.conn = nil
	ws.publishStatus("disconnected")
}

func dialHandshakeOK(response *http.Response, dialErr error) bool {
	return dialErr == nil &&
		response != nil &&
		response.StatusCode == http.StatusSwitchingProtocols
}

/*
Error returns the error of the Kraken public websocket.
*/
func (ws *WebSocket) Error() error {
	return ws.err
}

/*
Close closes the Kraken public websocket.
*/
func (ws *WebSocket) Close() (err error) {
	if ws.conn != nil {
		err = errnie.Guard(
			errnie.IO,
			"kraken/public: failed to close connection",
			errnie.Error(ws.conn.Close()),
		)
	}

	ws.cancel()
	return err
}

/*
Connect connects to the Kraken public websocket, using Fibonacci backoff.
It will return an error if the connection fails after the max delay.

The delay is calculated using the Fibonacci sequence:
1, 1, 2, 3, 5, 8, 13, 21, 34, 55, 89
*/
func (ws *WebSocket) Connect(endpoint EndpointType, n int) error {
	if n > ws.connectMaxDelay {
		return errnie.Error(errnie.Err(
			errnie.Unknown,
			"kraken/public: connect failed after max delay",
			fmt.Errorf("kraken/public: connect failed after %d seconds", n),
		))
	}

	if ws.isConnected.Load() && ws.conn != nil {
		return nil
	}

	var response *http.Response

	errnie.Info("kraken/public: websocket client dialing")

	ws.conn, response, ws.err = websocket.DefaultDialer.Dial(
		string(endpoint), http.Header{},
	)

	if dialHandshakeOK(response, ws.err) {
		ws.isConnected.Store(true)
		errnie.Info("kraken/public: websocket connected")
		ws.publishStatus("connected")

		return nil
	}

	if ws.err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"kraken/public: websocket dial failed",
			ws.err,
		))
	}

	if ws.err == nil && response != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"kraken/public: websocket handshake rejected",
			fmt.Errorf("status %d", response.StatusCode),
		))
	}

	time.Sleep(time.Duration(n) * time.Second)

	return ws.Connect(endpoint, int(
		math.Round((math.Pow(
			math.Phi, float64(n),
		)+math.Pow(
			math.Phi-1, float64(n),
		))/math.Sqrt(5)),
	))
}
