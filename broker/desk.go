package broker

import (
	"context"
	"strings"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
)

/*
Desk links trader actions to the private exchange channel.
It is deliberately small: balances, quotes, order encoding, and pending state are
composed objects so the desk does not become a second strategy engine.
*/
type Desk struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q[any]
	tree        *dmt.Tree
	private     *qpool.BroadcastGroup
	ui          *qpool.BroadcastGroup
	balances    *BalanceBook
	ticker      *Ticker
	factory     *OrderFactory
	pending     *PendingBook
	subscribers []*qpool.BroadcastConsumer
}

/*
NewDesk instantiates the broker execution seam.
*/
func NewDesk(ctx context.Context, pool *qpool.Q[any], tree *dmt.Tree) *Desk {
	ctx, cancel := context.WithCancel(ctx)
	desk := &Desk{
		ctx:      ctx,
		cancel:   cancel,
		pool:     pool,
		tree:     tree,
		balances: NewBalanceBook(),
		ticker:   NewTicker(),
		factory:  NewOrderFactory(),
		pending:  NewPendingBook(),
	}

	if pool != nil {
		desk.private = pool.CreateBroadcastGroup("kraken:private")
		desk.ui = pool.CreateBroadcastGroup("ui")
		for _, channel := range []string{"ticker", "executions", "balances"} {
			desk.subscribers = append(desk.subscribers, pool.Subscribe(channel, desk.onMessage))
		}
	}

	return desk
}

/*
Ready reports whether the desk has the objects required to dispatch actions.
*/
func (desk *Desk) Ready() bool {
	return desk != nil && desk.private != nil && desk.balances != nil &&
		desk.ticker != nil && desk.factory != nil && desk.pending != nil
}

/*
Update converts allowed trader actions into private add_order requests.
*/
func (desk *Desk) Update(actions []*datura.Artifact) error {
	if len(actions) == 0 {
		return nil
	}

	if !desk.Ready() {
		return errnie.Error(errnie.Err(errnie.Validation, "broker desk is not ready", nil))
	}

	for _, action := range actions {
		if !desk.actionAllowed(action) {
			desk.publishDiagnostic(action, "warning", "action not allowed for dispatch")
			continue
		}

		order, pending, err := desk.factory.Build(action, desk.balances, desk.ticker)
		if err != nil {
			desk.publishDiagnostic(action, "error", err.Error())
			continue
		}

		if !desk.pending.Add(pending) {
			order.Release()
			desk.publishDiagnostic(action, "warning", "duplicate pending order")
			continue
		}

		action.Poke(pending.ClOrdID, "cl_ord_id")
		desk.private.Send(order)
	}

	return nil
}

func (desk *Desk) actionAllowed(action *datura.Artifact) bool {
	if action == nil {
		return false
	}

	if strings.EqualFold(datura.Peek[string](action, "verdict"), "blocked") {
		return false
	}

	if strings.EqualFold(datura.Peek[string](action, "decision", "verdict"), "blocked") {
		return false
	}

	if !datura.Peek[bool](action, "allowed") {
		return false
	}

	if strings.EqualFold(actionString(action, "side"), "buy") &&
		!datura.Peek[bool](action, "risk", "stamped") {
		return false
	}

	return true
}

func (desk *Desk) onMessage(artifact *datura.Artifact) error {
	if desk == nil || artifact == nil || !artifact.IsValid() {
		return nil
	}

	destination := desk.destination(artifact)
	switch destination {
	case "balances":
		return desk.balances.Update(artifact)
	case "ticker":
		return desk.ticker.Update(artifact)
	case "executions", "orders":
		desk.pending.Update(artifact)
		return nil
	default:
		return nil
	}
}

func (desk *Desk) destination(artifact *datura.Artifact) string {
	destination, err := artifact.Destination()
	if err != nil {
		errnie.Error(errnie.Err(errnie.Validation, "desk: read destination", err))
	}

	if destination == "" || destination == "broker" {
		destination = datura.Peek[string](artifact, "role")
	}

	if destination == "" {
		destination = datura.Peek[string](artifact, "channel")
	}

	return strings.TrimSpace(destination)
}

func (desk *Desk) publishDiagnostic(
	action *datura.Artifact,
	severity string,
	reason string,
) {
	if desk == nil {
		return
	}

	symbol := actionSymbol(action)
	if symbol == "" {
		symbol = "broker"
	}

	payload := datura.Map[any]{
		"channel":       "broker",
		"type":          "diagnostic",
		"severity":      severity,
		"reason":        reason,
		"symbol":        symbol,
		"side":          actionString(action, "side"),
		"order_type":    actionString(action, "type"),
		"pending_count": desk.pending.Count(),
		"timestamp":     time.Now().UTC().Format(time.RFC3339Nano),
	}

	artifact := datura.Acquire("broker", datura.APPJSON).
		WithDestination("ui").
		WithRole("diagnostic").
		WithScope(symbol).
		WithPayload(payload.Marshal())

	if desk.ui != nil {
		desk.ui.Send(artifact)
	}

	if desk.tree != nil {
		if _, _, err := desk.tree.InsertArtifact(artifact.Prefix("role", "scope", "timestamp"), artifact); err != nil {
			errnie.Error(errnie.Err(errnie.Validation, "broker: insert diagnostic", err))
		}
	}
}

/*
Close releases desk subscriptions through context cancellation.
*/
func (desk *Desk) Close() error {
	if desk == nil {
		return nil
	}

	desk.cancel()
	return nil
}
