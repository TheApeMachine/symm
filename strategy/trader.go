package strategy

import (
	"context"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Trader is responsible for talking to the broker and managing positions.
*/
type Trader struct {
	*runtime.System
	desk *broker.Desk
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

func (trader *Trader) OnAction(
	label string,
	action cognition.Action,
	confidence float64,
) error {
	return nil
}
