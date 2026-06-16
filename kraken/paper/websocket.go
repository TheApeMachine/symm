package paper

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/paper/response"
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
	sockets         map[string]types.Socket
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
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  &sync.Map{},
		subscribers: &sync.Map{},
		sockets: map[string]types.Socket{
			"balances":   response.NewBalances(ctx, pool),
			"executions": response.NewExecutions(ctx, pool),
			"orders":     response.NewOrders(ctx, pool),
		},
		isConnected:     atomic.Bool{},
		connectMaxDelay: viper.GetInt("system.network.connection.max_delay"),
	}

	for _, channel := range []string{
		"balances", "executions", "orders", "kraken:socket",
	} {
		socket.broadcasts.Store(
			channel, socket.pool.CreateBroadcastGroup(channel),
		)
	}

	for _, channel := range []string{"kraken:socket", "kraken:private"} {
		socket.subscribers.Store(
			channel, pool.Subscribe(channel, socket.onMessage),
		)
	}

	errnie.Info("kraken/paper: websocket client ready")
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
			"kraken/paper: failed to get destination",
			err,
		))
	}).Value()

	switch destination {
	case "kraken:private":
		payload := errnie.Does(func() ([]byte, error) {
			return artifact.Payload()
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken/paper: failed to get payload",
				err,
			))
		}).Value()

		message := ws.sockets[artifact.Peek("role")].Send(payload)
		buffer, err := sonic.Marshal(message)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken/paper: failed to marshal message",
				err,
			))
		}

		out := datura.Acquire("kraken:private", datura.Artifact_Type_json)
		out.WithDestination("kraken:socket")
		out.WithRole(message.Channel)
		out.WithScope(message.Type)
		out.WithPayload(buffer)

		broadcast, ok := ws.broadcasts.Load("kraken:socket")

		if !ok {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken/paper: unknown channel",
				errors.New("kraken:socket"),
			))
		}

		broadcast.(*qpool.BroadcastGroup).Send(artifact)
	default:
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken/paper: ignored destination",
			errors.New(destination),
		))
	}

	return nil
}

/*
Run the Kraken public websocked read loop. This turns every message
into a datura.Artifact and sends it to the appropriate broadcast group.
*/
func (ws *WebSocket) Run() {
	for {
		select {
		case <-ws.ctx.Done():
			return
		default:
		}

		subscription, ok := ws.subscribers.Load("kraken:socket")

		if !ok {
			continue
		}

		consumer, ok := subscription.(*qpool.BroadcastConsumer)

		if !ok {
			continue
		}

		artifact, err := consumer.Wait(ws.ctx)

		if err != nil {
			errnie.Error(err)
			continue
		}

		message := datura.As[types.SocketMessage](artifact)

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
				"kraken/paper: error",
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
Error returns the error of the Kraken paper websocket.
*/
func (ws *WebSocket) Error() error {
	return ws.err
}

/*
Close closes the Kraken paper websocket.
*/
func (ws *WebSocket) Close() (err error) {
	ws.cancel()
	return err
}

/*
Connect connects to the Kraken paper websocket, using Fibonacci backoff.
It will return an error if the connection fails after the max delay.

The delay is calculated using the Fibonacci sequence:
1, 1, 2, 3, 5, 8, 13, 21, 34, 55, 89
*/
func (ws *WebSocket) Connect(n int) error {
	if n > ws.connectMaxDelay {
		return errnie.Error(errnie.Err(
			errnie.Unknown,
			"kraken/paper: connect failed after max delay",
			fmt.Errorf("kraken/paper: connect failed after %d seconds", n),
		))
	}

	if ws.isConnected.Load() {
		return nil
	}

	return ws.Connect(n)
}
