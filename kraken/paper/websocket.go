package paper

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/frame"
	"github.com/theapemachine/symm/kraken/paper/response"
	"github.com/theapemachine/symm/kraken/types"
)

/*
WebSocket simulates Kraken private channels for paper trading.
*/
type WebSocket struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	tree        *dmt.Tree
	uiBroadcast *qpool.BroadcastGroup
	broadcasts  *sync.Map
	subscribers *sync.Map
	sockets     map[string]types.Socket
	armed       atomic.Bool
}

/*
NewWebSocket creates a new Kraken paper private channel simulator.
*/
func NewWebSocket(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *WebSocket {
	ctx, cancel := context.WithCancel(ctx)

	balances := response.NewBalances(ctx, pool, tree)
	executions := response.NewExecutions(ctx, pool, tree)
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
			if frame.AckOnlyRequest(payload) {
				return nil
			}

			return errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken/paper: private channel returned no message",
				errors.New(channelRole),
			))
		}

		return errnie.Error(frame.Publish(ws.tree, ws.uiBroadcast, payload, message))
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
	if err := ws.arm(); err != nil {
		panic(err)
	}
	<-ws.ctx.Done()
}

/*
Arm synchronously subscribes the paper private channels. Root calls this before
starting the trader, so balances/executions/orders are active before any paper
order can publish a fill.
*/
func (ws *WebSocket) Arm() error {
	return ws.arm()
}

func (ws *WebSocket) arm() error {
	if ws.armed.Swap(true) {
		return nil
	}

	for _, channelRole := range []string{"balances", "executions", "orders"} {
		params := map[string]any{"channel": channelRole}

		if channelRole == "balances" {
			params["snapshot"] = true
		}

		request, buildErr := types.NewKrakenMessage("subscribe", params, 0)

		if buildErr != nil {
			return errnie.Err(errnie.Validation, "kraken/paper: build subscribe "+channelRole, buildErr)
		}

		payload, marshalErr := sonic.Marshal(request)

		if marshalErr != nil {
			return errnie.Err(errnie.Validation, "kraken/paper: marshal subscribe "+channelRole, marshalErr)
		}

		artifact := datura.Acquire("paper", datura.APPJSON).
			WithDestination("kraken:private").
			WithRole(channelRole).
			WithPayload(payload)

		if err := ws.onMessage(artifact); err != nil {
			return errnie.Err(errnie.Validation, "kraken/paper: publish subscribe "+channelRole, err)
		}
	}

	return nil
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
