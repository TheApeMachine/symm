package broker

import (
	"context"
	"strings"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	dashboard "github.com/theapemachine/symm/ui"
)

/*
Desk links trader actions to the private exchange channel.
It is deliberately small: balances, quotes, order encoding, and pending state are
composed objects so the desk does not become a second strategy engine.
*/
type Desk struct {
	ctx       context.Context
	cancel    context.CancelFunc
	account   Account
	publisher Publisher
	balances  *BalanceBook
	ticker    *Ticker
	factory   *OrderFactory
	capital   *Capital
	pending   *PendingBook
}

type Account interface {
	Submit(*websocket.OrderRequest) error
}

type Publisher interface {
	Publish(dashboard.Message) error
}

/*
NewDesk instantiates the broker execution seam.
*/
func NewDesk(
	ctx context.Context,
	account Account,
	publisher Publisher,
) (*Desk, error) {
	ctx, cancel := context.WithCancel(ctx)

	if account == nil {
		cancel()
		return nil, errnie.Error(errnie.Err(errnie.Validation, "broker: account submitter required", nil))
	}

	if publisher == nil {
		cancel()
		return nil, errnie.Error(errnie.Err(errnie.Validation, "broker: dashboard publisher required", nil))
	}

	factory := NewOrderFactory()
	return &Desk{
		ctx:       ctx,
		cancel:    cancel,
		account:   account,
		publisher: publisher,
		balances:  NewBalanceBook(),
		ticker:    NewTicker(),
		factory:   factory,
		capital:   NewCapital(factory.quote),
		pending:   NewPendingBook(),
	}, nil
}

/*
Ready reports whether the desk has the objects required to dispatch actions.
*/
func (desk *Desk) Ready() bool {
	return desk.account != nil && desk.balances != nil &&
		desk.ticker != nil && desk.factory != nil &&
		desk.capital != nil && desk.pending != nil
}

/*
Update converts allowed trader actions into private add_order requests.
*/
func (desk *Desk) Update(actions []*logic.Action) error {
	if len(actions) == 0 {
		return nil
	}

	if !desk.Ready() {
		return errnie.Error(errnie.Err(errnie.Validation, "broker desk is not ready", nil))
	}

	desk.capital.Reset()
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

		if err := desk.capital.Reserve(pending, desk.balances); err != nil {
			desk.publishDiagnostic(action, "error", err.Error())
			continue
		}

		if !desk.pending.Add(pending) {
			desk.publishDiagnostic(action, "warning", "duplicate pending order")
			continue
		}

		action.ClOrdID = pending.ClOrdID
		if err := desk.account.Submit(order); err != nil {
			desk.publishDiagnostic(action, "error", err.Error())
			return err
		}
	}

	return nil
}

func (desk *Desk) actionAllowed(action *logic.Action) bool {
	if action == nil {
		return false
	}

	if strings.EqualFold(action.Verdict, "blocked") {
		return false
	}

	if !action.Allowed {
		return false
	}

	if action.Side == logic.SideBuy && !action.RiskStamped {
		return false
	}

	return true
}

func (desk *Desk) Observe(frame map[string]any) error {
	return desk.onMessage(frame)
}

func (desk *Desk) onMessage(frame map[string]any) error {
	if len(frame) == 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "broker: frame is empty", nil))
	}

	destination := desk.destination(frame)
	switch destination {
	case "balances":
		return desk.balances.Update(frame)
	case "ticker":
		return desk.ticker.Update(frame)
	case "executions", "orders":
		return desk.pending.Update(frame)
	default:
		return errnie.Error(errnie.Err(errnie.Validation, "broker: unsupported frame channel: "+destination, nil))
	}
}

func (desk *Desk) destination(frame map[string]any) string {
	destination := stringValue(frame["channel"])
	if destination == "" {
		destination = stringValue(frame["type"])
	}

	return strings.TrimSpace(destination)
}

func (desk *Desk) publishDiagnostic(
	action *logic.Action,
	severity string,
	reason string,
) {
	symbol := actionSymbol(action)
	if symbol == "" {
		symbol = "broker"
	}

	errnie.Error(desk.publisher.Publish(dashboard.Message{
		Diagnostic: &dashboard.Diagnostic{
			Severity: severity,
			Reason:   reason,
			Symbol:   symbol,
			At:       time.Now().UTC().Format(time.RFC3339Nano),
		},
	}))
}

func (desk *Desk) Holdings() (*logic.Holdings, error) {
	return desk.balances.Holdings()
}

func (desk *Desk) Quote(symbol string) (MarketQuote, bool) {
	return desk.ticker.Quote(symbol)
}

/*
Close releases desk subscriptions through context cancellation.
*/
func (desk *Desk) Close() error {
	desk.cancel()
	return nil
}
