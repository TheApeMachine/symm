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
	conn            *websocket.Conn
	isConnected     atomic.Bool
	connectMaxDelay int
}

/*
NewWebSocket creates a new Kraken public websocket client.
*/
func NewWebSocket(
	ctx context.Context, pool *qpool.Q[any], tree *dmt.Tree,
) *WebSocket {
	ctx, cancel := context.WithCancel(ctx)

	socket := &WebSocket{
		ctx:             ctx,
		cancel:          cancel,
		tree:            tree,
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

		ws.conn.WriteMessage(websocket.TextMessage, payload)
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
			errnie.Error(ws.Connect(endpoint, 1))
			continue
		}

		var payload []byte

		if _, payload, ws.err = ws.conn.ReadMessage(); ws.err != nil {
			ws.isConnected.Store(false)
			continue
		}

		artifact := datura.Acquire(
			"kraken:public", datura.Artifact_Type_json,
		).WithDestination(
			"kraken:public",
		).WithPayload(payload)

		channel := datura.PeekPayload[string](artifact, "channel")

		switch channel {
		case BookChannel, TradesChannel, TickerChannel, CandlesChannel:
			symbol, ok := datura.PeekPayloadOK[string](artifact, "data.0.symbol")
			if !ok || symbol == "" {
				artifact.Release()
				continue
			}

			artifact.WithRole(channel).WithScope(symbol)
		case InstrumentsChannel:
			artifact.WithRole(channel).WithScope(
				datura.PeekPayload[string](artifact, "symbol"),
			)
		default:
			symbol := datura.PeekPayload[string](artifact, "symbol")
			artifact.WithRole(channel).WithScope(symbol)
		}

		ws.tree.Insert(artifact.Prefix(), artifact.Marshal())
		artifact.Release()
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

	if ws.err == nil && response != nil && response.StatusCode == http.StatusSwitchingProtocols {
		ws.isConnected.Store(true)
		errnie.Info("kraken/public: websocket connected")
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
