package private

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/frame"
	"github.com/theapemachine/symm/kraken/types"
)

/*
WebSocket is the Kraken private websocket client.
*/
type WebSocket struct {
	ctx             context.Context
	cancel          context.CancelFunc
	err             error
	pool            *qpool.Q[any]
	tree            *dmt.Tree
	uiBroadcast     *qpool.BroadcastGroup
	broadcasts      *sync.Map
	subscribers     *sync.Map
	conn            *websocket.Conn
	endpoint        string
	isConnected     atomic.Bool
	connectMaxDelay int
}

/*
NewWebSocket creates a new Kraken private websocket client.
*/
func NewWebSocket(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *WebSocket {
	ctx, cancel := context.WithCancel(ctx)

	socket := &WebSocket{
		ctx:             ctx,
		cancel:          cancel,
		pool:            pool,
		tree:            tree,
		uiBroadcast:     pool.CreateBroadcastGroup("ui"),
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
	case "kraken:private":
		if ws.conn == nil || !ws.isConnected.Load() {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken/private: websocket is not connected",
				nil,
			))
		}

		payload := errnie.Does(func() ([]byte, error) {
			return artifact.DecryptPayload(), nil
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken/private: failed to get payload",
				err,
			))
		}).Value()

		payload, tokenErr := ws.payloadWithToken(payload)

		if tokenErr != nil {
			return errnie.Error(tokenErr)
		}

		return errnie.Error(ws.conn.WriteMessage(websocket.TextMessage, payload))
	default:
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken/private: ignored destination",
			errors.New(destination),
		))
	}
}

/*
Run reads Kraken private websocket frames and routes them into broadcast groups.
*/
func (ws *WebSocket) Run() {
	for {
		select {
		case <-ws.ctx.Done():
			return
		default:
		}

		if !ws.isConnected.Load() || ws.conn == nil {
			if ws.endpoint == "" {
				time.Sleep(time.Second)
				continue
			}

			errnie.Error(ws.Connect(ws.endpoint, 1))
			continue
		}

		var payload []byte
		message := types.Acquire()

		if _, payload, ws.err = ws.conn.ReadMessage(); ws.err != nil {
			message.Release()
			ws.isConnected.Store(false)

			if ws.conn != nil {
				_ = ws.conn.Close()
				ws.conn = nil
			}

			continue
		}

		if ws.err = errnie.Error(
			message.Decode(payload),
		); ws.err != nil {
			message.Release()
			continue
		}

		if ws.err = errnie.Error(ws.publish(payload, message)); ws.err != nil {
			message.Release()
			continue
		}

		message.Release()
	}
}

func (ws *WebSocket) payloadWithToken(payload []byte) ([]byte, error) {
	var envelope map[string]any

	if err := sonic.Unmarshal(payload, &envelope); err != nil {
		return nil, fmt.Errorf("kraken/private: decode request: %w", err)
	}

	params, ok := envelope["params"].(map[string]any)

	if !ok {
		params = make(map[string]any)
	}

	if _, ok = params["token"]; ok {
		return payload, nil
	}

	token, err := types.NewToken(ws.ctx)

	if err != nil {
		return nil, fmt.Errorf("kraken/private: token: %w", err)
	}

	params["token"] = token
	envelope["params"] = params

	return sonic.Marshal(envelope)
}

func (ws *WebSocket) publish(
	payload []byte,
	message *types.SocketMessage,
) error {
	return frame.Publish(ws.tree, ws.uiBroadcast, payload, message)
}

/*
Error returns the error of the Kraken private websocket.
*/
func (ws *WebSocket) Error() error {
	return ws.err
}

/*
Close closes the Kraken private websocket.
*/
func (ws *WebSocket) Close() (err error) {
	if ws.conn != nil {
		err = errnie.Guard(
			errnie.IO,
			"kraken/private: failed to close connection",
			errnie.Error(ws.conn.Close()),
		)
	}

	ws.cancel()
	return err
}

/*
Connect connects to the Kraken private websocket, using Fibonacci backoff.
It will return an error if the connection fails after the max delay.
*/
func (ws *WebSocket) Connect(endpoint string, n int) error {
	if endpoint != "" {
		ws.endpoint = endpoint
	}

	if n > ws.connectMaxDelay {
		return errnie.Error(errnie.Err(
			errnie.Unknown,
			"kraken/private: connect failed after max delay",
			fmt.Errorf("kraken/private: connect failed after %d seconds", n),
		))
	}

	if ws.isConnected.Load() && ws.conn != nil {
		return nil
	}

	if _, tokenErr := types.NewToken(ws.ctx); tokenErr != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken/private: failed to acquire websocket token",
			tokenErr,
		))
	}

	var response *http.Response

	ws.conn, response, ws.err = websocket.DefaultDialer.Dial(
		ws.endpoint, http.Header{},
	)

	if ws.err == nil && response != nil && response.StatusCode == http.StatusSwitchingProtocols {
		ws.isConnected.Store(true)
		return nil
	}

	time.Sleep(time.Duration(n) * time.Second)

	return ws.Connect(ws.endpoint, int(
		math.Round((math.Pow(
			math.Phi, float64(n),
		)+math.Pow(
			math.Phi-1, float64(n),
		))/math.Sqrt(5)),
	))
}
