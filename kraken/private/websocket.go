package private

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
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
	tree            *dmt.Tree
	uiBroadcast     *qpool.BroadcastGroup
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
	role := ws.channel(payload, message)

	if role == "" {
		return nil
	}

	output := datura.Acquire("kraken:private", datura.APPJSON).
		WithDestination("ui").
		WithRole(role).
		WithPayload(ws.payload(role, payload, message))

	if message.Type != "" {
		output.WithScope(message.Type)
	}

	if ws.tree != nil {
		ws.tree.Insert(output.Prefix(), output.Pack())
	}

	return ws.uiBroadcast.Send(output)
}

func (ws *WebSocket) channel(
	payload []byte,
	message *types.SocketMessage,
) string {
	if message != nil && strings.TrimSpace(message.Channel) != "" {
		return strings.TrimSpace(message.Channel)
	}

	var envelope struct {
		Channel string `json:"channel"`
		Result  struct {
			Channel string `json:"channel"`
		} `json:"result"`
		Params struct {
			Channel string `json:"channel"`
		} `json:"params"`
	}

	if sonic.Unmarshal(payload, &envelope) != nil {
		return ""
	}

	if strings.TrimSpace(envelope.Channel) != "" {
		return strings.TrimSpace(envelope.Channel)
	}

	if strings.TrimSpace(envelope.Result.Channel) != "" {
		return strings.TrimSpace(envelope.Result.Channel)
	}

	return strings.TrimSpace(envelope.Params.Channel)
}

func (ws *WebSocket) payload(
	role string,
	payload []byte,
	message *types.SocketMessage,
) []byte {
	if message == nil || len(message.Data) == 0 {
		return payload
	}

	switch role {
	case "balances":
		return ws.wrap("asset", message.Data)
	case "executions":
		return ws.wrap("executions", message.Data)
	}

	if json.Valid(message.Data) && message.Data[0] == '{' {
		return message.Data
	}

	return ws.wrap("data", message.Data)
}

func (ws *WebSocket) wrap(key string, value json.RawMessage) []byte {
	payload, err := sonic.Marshal(map[string]json.RawMessage{
		key: value,
	})

	if err != nil {
		return []byte(`{}`)
	}

	return payload
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

	if _, tokenErr := types.NewToken(ws.ctx); tokenErr != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken/private: failed to acquire websocket token",
			tokenErr,
		))
	}

	var response *http.Response

	ws.conn, response, ws.err = websocket.DefaultDialer.Dial(
		string(endpoint), http.Header{},
	)

	if ws.err == nil && response != nil && response.StatusCode == http.StatusSwitchingProtocols {
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
