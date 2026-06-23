package paper

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
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
	tree            *dmt.Tree
	uiBroadcast     *qpool.BroadcastGroup
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
	tree *dmt.Tree,
) *WebSocket {
	ctx, cancel := context.WithCancel(ctx)

	balances := response.NewBalances(ctx, pool)
	executions := response.NewExecutions(ctx, pool)
	orders := response.NewOrdersWithTree(ctx, pool, tree, balances, executions)

	socket := &WebSocket{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		tree:        tree,
		uiBroadcast: pool.CreateBroadcastGroup("ui"),
		broadcasts:  &sync.Map{},
		subscribers: &sync.Map{},
		sockets: map[string]types.Socket{
			"balances":   balances,
			"executions": executions,
			"orders":     orders,
		},
		isConnected:     atomic.Bool{},
		connectMaxDelay: viper.GetInt("system.network.connection.max_delay"),
	}

	for _, channelName := range []string{
		"balances", "executions", "orders",
	} {
		socket.broadcasts.Store(
			channelName, socket.pool.CreateBroadcastGroup(channelName),
		)
	}

	socket.subscribers.Store(
		"kraken:private", pool.Subscribe("kraken:private", socket.onMessage),
	)

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
			return artifact.DecryptPayload(), nil
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken/paper: failed to get payload",
				err,
			))
		}).Value()

		channelRole := errnie.Does(func() (string, error) {
			return artifact.Role()
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken/paper: failed to get role",
				err,
			))
		}).Value()

		socket, ok := ws.sockets[channelRole]

		if !ok || socket == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken/paper: unknown private channel",
				errors.New(channelRole),
			))
		}

		message := socket.Send(payload)

		if message == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken/paper: private channel returned no message",
				errors.New(channelRole),
			))
		}

		output := datura.Acquire(
			"kraken:private", datura.APPJSON,
		).WithDestination(
			"ui",
		).WithPayload(
			message.Data,
		).WithRole(
			channelRole,
		)

		ws.tree.Insert(output.Prefix(), output.Pack())
		return errnie.Error(ws.uiBroadcast.Send(output))
	default:
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken/paper: ignored destination",
			errors.New(destination),
		))
	}
}

/*
Run arms Kraken private paper channels and keeps the subscribe handler alive.
*/
func (ws *WebSocket) Run() {
	<-ws.ctx.Done()
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
