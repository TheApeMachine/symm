package strategy

import (
	"context"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Trader is responsible for talking to the broker and managing positions.
*/
type Trader struct {
	*runtime.System
	desk      *broker.Desk
	positions map[string]*broker.Position
}

func NewTrader(ctx context.Context, api *websocket.API) *Trader {
	return &Trader{
		System:    runtime.NewSystem(ctx, "trader"),
		desk:      broker.NewDesk(ctx, api),
		positions: make(map[string]*broker.Position),
	}
}

func (trader *Trader) OnAction(symbol string, action Action) {
	switch action {
	case ActionEnter:
		if pos := trader.desk.Enter(symbol); pos != nil {
			trader.positions[symbol] = pos
		}
	case ActionExit:
		if pos, ok := trader.positions[symbol]; ok {
			trader.desk.Exit(pos)
			delete(trader.positions, symbol)
		}
	}
}

func (trader *Trader) Position(symbol string) *broker.Position {
	return trader.positions[symbol]
}

func (trader *Trader) Holding(symbol string) bool {
	return trader.positions[symbol] != nil
}
