package public

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public/response"
	"github.com/theapemachine/symm/kraken/types"
)

/*
WebSocket is the Kraken public websocket client.
*/
type WebSocket struct {
	ctx             context.Context
	cancel          context.CancelFunc
	err             error
	tree            *dmt.Tree
	pool            *qpool.Q[any]
	broadcasts      *sync.Map
	subscribers     *sync.Map
	dialer          *websocket.Dialer
	conn            *websocket.Conn
	isConnected     atomic.Bool
	connectMaxDelay int
	instrument      *Instrument
	destination     string
	handlers        map[string]types.Socket
	token           *Token
}

/*
NewWebSocket creates a new Kraken public websocket client.
*/
func NewWebSocket(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
	dialer *websocket.Dialer,
	broadcasts []string,
	subscriptions []string,
) *WebSocket {
	ctx, cancel := context.WithCancel(ctx)

	treeHandler := response.NewTreeHandler(tree)
	instrument := NewInstrument(ctx, pool)
	destination := subscriptions[0]

	socket := &WebSocket{
		ctx:             ctx,
		cancel:          cancel,
		tree:            tree,
		pool:            pool,
		broadcasts:      &sync.Map{},
		subscribers:     &sync.Map{},
		dialer:          dialer,
		isConnected:     atomic.Bool{},
		connectMaxDelay: viper.GetInt("system.network.connection.max_delay"),
		instrument:      instrument,
		destination:     destination,
		handlers: map[string]types.Socket{
			"balances":   treeHandler,
			"executions": treeHandler,
			"instrument": instrument,
			"orders":     treeHandler,
			"ticker":     treeHandler,
			"trade":      treeHandler,
			"trades":     treeHandler,
			"ohlc":       treeHandler,
			"book":       treeHandler,
			"level3":     treeHandler,
		},
		token: NewToken(ctx, destination),
	}

	for _, channel := range broadcasts {
		bg := pool.CreateBroadcastGroup(channel)
		socket.broadcasts.Store(channel, bg)

		broadcastHandler := response.NewBroadcastHandler(
			[]string{channel}, destination, channel, bg,
		)

		if channel == "desk" {
			broadcastHandler = response.NewBroadcastHandler(
				[]string{"ticker"}, destination, "desk", bg,
			)
		}

		treeHandler.Observe(broadcastHandler)
	}

	for _, channel := range subscriptions {
		socket.subscribers.Store(
			channel, pool.Subscribe(channel, socket.onMessage),
		)
	}

	errnie.Info(socket.destination + ": websocket client ready")
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
			ws.destination+": failed to get destination",
			err,
		))
	}).Value()

	switch destination {
	case ws.destination:
		if ws.destination == "kraken:private" {
			role := datura.Peek[string](artifact, "role")

			if _, ok := ws.broadcasts.Load(role); !ok {
				return nil
			}
		}

		if ws.conn == nil || !ws.isConnected.Load() {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				ws.destination+": websocket not connected",
				errors.New("not connected"),
			))
		}

		return errnie.Error(ws.conn.WriteMessage(
			websocket.TextMessage, ws.token.Wrap(artifact),
		))
	default:
		return errnie.Error(errnie.Err(
			errnie.Validation,
			ws.destination+": ignored destination",
			errors.New(destination),
		))
	}
}

/*
Run reads Kraken websocket messages and calls the appropriate handler
keyed by the channel (role) of the message. Handlers use an observer
pattern for any additional side-effects.
*/
func (ws *WebSocket) Run(endpoint EndpointType) {
	errnie.Info(ws.destination + ": websocket client running")

	for {
		select {
		case <-ws.ctx.Done():
			return
		default:
		}

		if !ws.isConnected.Load() || ws.conn == nil {
			if err := ws.Connect(endpoint, 1); err != nil {
				errnie.Error(err)
				continue
			}

			if ws.destination == "kraken:public" {
				go func() {
					errnie.Error(ws.instrument.Subscribe())
				}()
			}

			continue
		}

		_, message, err := ws.conn.ReadMessage()

		if err != nil {
			if errnie.Error(ws.ctx.Err()) != nil {
				return
			}

			ws.err = errnie.Error(errnie.Err(
				errnie.IO,
				ws.destination+": websocket read failed",
				err,
			))
			ws.disconnect()
			continue
		}

		msg := &types.SocketMessage{}

		if err := msg.Decode(message); err != nil {
			ws.err = errnie.Error(err)
			continue
		}

		handler, ok := ws.handlers[msg.Channel]

		if !ok || handler == nil {
			continue
		}

		handler.Send(message)
	}
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
	ws.disconnect()
	ws.cancel()
	return
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
			ws.destination+": connect failed after max delay",
			fmt.Errorf(ws.destination+": connect failed after %d seconds", n),
		))
	}

	if ws.isConnected.Load() && ws.conn != nil {
		ws.instrument.Send(datura.Map[any]{
			"method": "subscribe",
			"params": datura.Map[any]{
				"channel": "instrument",
			},
			"req_id": time.Now().UTC().UnixNano(),
		}.Marshal())
		return nil
	}

	var response *http.Response

	errnie.Info(ws.destination + ": websocket client dialing")

	ws.conn, response, ws.err = ws.dialer.Dial(
		string(endpoint), http.Header{},
	)

	if ws.err == nil && response != nil && response.StatusCode == http.StatusSwitchingProtocols {
		ws.isConnected.Store(true)
		errnie.Info(ws.destination + ": websocket connected")
		return nil
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

func (ws *WebSocket) disconnect() {
	ws.isConnected.Store(false)

	if ws.conn != nil {
		errnie.Error(ws.conn.Close())
	}

	ws.conn = nil
}
