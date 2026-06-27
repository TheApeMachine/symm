package private

import (
	"context"
	"errors"
	"fmt"
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

var dialWebSocket = websocket.DefaultDialer.Dial

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

		return errnie.Error(ws.send(payload))
	default:
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken/private: ignored destination",
			errors.New(destination),
		))
	}
}

func (ws *WebSocket) send(payload []byte) error {
	if ws == nil || ws.conn == nil || !ws.isConnected.Load() {
		return errnie.Err(errnie.Validation, "kraken/private: websocket is not connected", nil)
	}

	return ws.conn.WriteMessage(websocket.TextMessage, payload)
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

			if err := ws.Connect(ws.endpoint, 1); err != nil {
				if ws.ctx.Err() != nil {
					return
				}
				ws.err = errnie.Err(errnie.IO, "kraken/private: websocket connect failed", err)
				errnie.Error(ws.err)
				continue
			}
		}

		if err := ws.readOnce(); err != nil {
			ws.err = err
			ws.disconnect()
			errnie.Error(err)
		}
	}
}

func (ws *WebSocket) readOnce() error {
	message := types.Acquire()
	defer message.Release()

	_, payload, err := ws.conn.ReadMessage()
	if err != nil {
		return errnie.Err(errnie.IO, "kraken/private: failed to read message", err)
	}

	if err := message.Decode(payload); err != nil {
		return errnie.Err(errnie.IO, "kraken/private: decode message", err)
	}

	if err := ws.publish(payload, message); err != nil {
		return errnie.Err(errnie.IO, "kraken/private: publish message", err)
	}

	return nil
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
	if err := frame.Publish(ws.tree, ws.uiBroadcast, payload, message); err != nil {
		return err
	}

	role := frame.ChannelRole(payload, message)
	if role == "" {
		return nil
	}

	raw, buildErr := frame.Artifact(payload, message)
	if buildErr != nil {
		return buildErr
	}
	if raw == nil {
		return nil
	}

	bg, ok := ws.broadcasts.Load(role)
	if !ok {
		return errnie.Err(errnie.Validation, "kraken/private: missing broadcast for "+role, nil)
	}

	return bg.(*qpool.BroadcastGroup).Send(raw)
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

	ws.isConnected.Store(false)
	ws.cancel()
	return err
}

/*
Connect connects to the Kraken private websocket using the configured bounded
reconnect delay. The signature stays stable for existing callers; the attempt
argument is ignored by the iterative implementation.
*/
func (ws *WebSocket) Connect(endpoint string, _ int) error {
	if endpoint != "" {
		ws.endpoint = endpoint
	}

	if ws.isConnected.Load() && ws.conn != nil {
		return nil
	}

	delay := ws.reconnectInitial()
	maxDelay := ws.reconnectMax()

	for {
		if err := ws.ctx.Err(); err != nil {
			return err
		}

		if _, tokenErr := types.NewToken(ws.ctx); tokenErr != nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken/private: failed to acquire websocket token",
				tokenErr,
			))
		}

		conn, response, err := dialWebSocket(ws.endpoint, http.Header{})
		ws.err = err

		if err == nil && response != nil && response.StatusCode == http.StatusSwitchingProtocols {
			ws.conn = conn
			ws.isConnected.Store(true)
			return nil
		}

		if conn != nil {
			_ = conn.Close()
		}
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		ws.err = errnie.Err(
			errnie.IO,
			"kraken/private: websocket dial failed",
			err,
		).With("status", status)

		if !sleepContext(ws.ctx, delay) {
			return ws.ctx.Err()
		}
		delay = nextReconnectDelay(delay, maxDelay, ws.reconnectMultiplier())
	}
}

func (ws *WebSocket) disconnect() {
	ws.isConnected.Store(false)
	if ws.conn != nil {
		_ = ws.conn.Close()
		ws.conn = nil
	}
}

func (ws *WebSocket) reconnectInitial() time.Duration {
	delay := viper.GetDuration("market.ws_reconnect_initial")
	if delay <= 0 {
		delay = time.Second
	}
	return delay
}

func (ws *WebSocket) reconnectMax() time.Duration {
	delay := viper.GetDuration("market.ws_reconnect_max")
	if delay <= 0 {
		if ws.connectMaxDelay > 0 {
			delay = time.Duration(ws.connectMaxDelay) * time.Second
		} else {
			delay = 30 * time.Second
		}
	}
	return delay
}

func (ws *WebSocket) reconnectMultiplier() float64 {
	multiplier := viper.GetFloat64("market.ws_reconnect_multiplier")
	if multiplier < 1 {
		multiplier = 2
	}
	return multiplier
}

func nextReconnectDelay(current, maxDelay time.Duration, multiplier float64) time.Duration {
	if maxDelay <= 0 {
		return current
	}
	next := time.Duration(float64(current) * multiplier)
	if next <= current {
		next = current
	}
	if next > maxDelay {
		return maxDelay
	}
	return next
}

func sleepContext(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
