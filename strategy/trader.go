package strategy

import (
	"context"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/broker/position"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Trader is responsible for talking to the broker and managing positions.
*/
type Trader struct {
	*runtime.System
	desk *broker.Desk
	mu   sync.Mutex
	open map[string]*position.Regulator
}

func NewTrader(
	ctx context.Context,
	private broker.Transport,
	price *broker.Price,
	balance *broker.Balance,
) *Trader {
	trader := &Trader{
		System: runtime.NewSystem(ctx, "trader"),
		desk:   broker.NewDesk(ctx, private, price, balance),
	}

	return trader
}

func (trader *Trader) Position(symbol string) *position.Regulator {
	if trader == nil {
		return nil
	}

	trader.mu.Lock()
	defer trader.mu.Unlock()

	return trader.open[symbol]
}

func (trader *Trader) OnAction(label string, action cognition.Action, confidence float64) error {
	if trader == nil || trader.desk == nil || trader.desk.Execution == nil || label == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: execution is not available",
			nil,
		))
	}

	if action == cognition.ActionExit {
		return trader.exit(label)
	}

	if action == cognition.ActionEnter {
		return trader.enter(label)
	}

	return nil
}

func (trader *Trader) enter(symbol string) error {
	current := trader.Position(symbol)

	if current != nil && !current.IsClosed() {
		return nil
	}

	regulator, err := trader.desk.Execution.Enter(symbol)

	if err != nil {
		return err
	}

	trader.mu.Lock()
	defer trader.mu.Unlock()

	if trader.open == nil {
		trader.open = make(map[string]*position.Regulator)
	}

	trader.open[symbol] = regulator

	return nil
}

func (trader *Trader) exit(symbol string) error {
	current := trader.Position(symbol)

	if current == nil || !current.IsHolding() || current.Status() == "exit_pending" {
		return nil
	}

	if err := trader.desk.Execution.Exit(current); err != nil {
		return err
	}

	if current.IsClosed() {
		trader.mu.Lock()
		delete(trader.open, symbol)
		trader.mu.Unlock()
	}

	return nil
}
