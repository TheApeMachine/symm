package private

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
	ctx context.Context,
	pool *qpool.Q[any],
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
		"balances", "executions", "orders",
	} {
		socket.broadcasts.Store(
			channel, socket.pool.CreateBroadcastGroup(channel),
		)
	}

	for _, channel := range []string{"kraken:private"} {
		socket.subscribers.Store(
			channel, pool.Subscribe(channel, socket.onMessage),
		)
	}

	errnie.Info("kraken/private: websocket client ready")
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
			"kraken/private: failed to get destination",
			err,
		))
	}).Value()

	switch destination {
	case "kraken:public":
		payload := errnie.Does(func() ([]byte, error) {
			return artifact.Payload()
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken/private: failed to get payload",
				err,
			))
		}).Value()

		ws.conn.WriteMessage(websocket.TextMessage, payload)
	default:
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken/private: ignored destination",
			errors.New(destination),
		))
	}

	return nil
}

/*
Run the Kraken private websocked read loop. This turns every message
into a datura.Artifact and sends it to the appropriate broadcast group.
*/
func (ws *WebSocket) Run() {
	for {
		select {
		case <-ws.ctx.Done():
			return
		default:
		}

		var payload []byte
		message := types.Acquire()

		if _, payload, ws.err = ws.conn.ReadMessage(); ws.err != nil {
			message.Release()
			continue
		}

		if ws.err = errnie.Error(
			message.Decode(payload),
		); ws.err != nil {
			message.Release()
			continue
		}

		artifact := datura.Acquire("kraken:private", datura.Artifact_Type_json)

		artifact.WithRole(
			message.Channel,
		).WithScope(
			message.Type,
		).WithPayload(
			message.Data,
		).Poke(
			"success", strconv.FormatBool(message.Success),
		).Poke(
			"time_in", message.TimeIn.Format(time.RFC3339),
		).Poke(
			"time_out", message.TimeOut.Format(time.RFC3339),
		)

		if message.Error != "" {
			artifact.WithError(errnie.Err(
				errnie.Unknown,
				"kraken/private: error",
				errors.New(message.Error),
			))
		}

		if bg, ok := ws.broadcasts.Load(artifact.Peek("role")); ok {
			bg.(*qpool.BroadcastGroup).Send(artifact)
		}

		message.Release()
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
func (ws *WebSocket) Connect(endpoint string, n int) error {
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

	ws.conn, response, ws.err = websocket.DefaultDialer.Dial(
		string(endpoint), http.Header{},
	)

	if ws.err == nil && response.StatusCode == http.StatusSwitchingProtocols {
		ws.isConnected.Store(true)
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
