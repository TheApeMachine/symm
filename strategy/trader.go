package strategy

import (
	"context"
	"sync"

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
	balance   *broker.Balance
	positions map[string]*broker.Position
	mu        sync.RWMutex
}

func NewTrader(
	ctx context.Context,
	api *websocket.API,
	price *broker.Price,
	balance *broker.Balance,
) *Trader {
	return &Trader{
		System:    runtime.NewSystem(ctx, "trader"),
		desk:      broker.NewDesk(ctx, api, price, balance),
		balance:   balance,
		positions: make(map[string]*broker.Position),
	}
}

func (trader *Trader) OnAction(symbol string, action Action) {
	if trader == nil || symbol == "" {
		return
	}

	trader.mu.Lock()
	defer trader.mu.Unlock()

	switch action {
	case ActionEnter:
		if trader.positions[symbol] != nil {
			return
		}

		position := trader.desk.Enter(symbol)

		if position != nil {
			trader.positions[symbol] = position
		}
	case ActionExit:
		position, found := trader.positions[symbol]

		if found && position != nil {
			trader.desk.Exit(position)
			delete(trader.positions, symbol)
		}
	}
}

func (trader *Trader) Position(symbol string) *broker.Position {
	if trader == nil {
		return nil
	}

	trader.mu.RLock()
	defer trader.mu.RUnlock()

	return trader.positions[symbol]
}

func (trader *Trader) Holding(symbol string) bool {
	if trader == nil {
		return false
	}

	trader.mu.RLock()
	defer trader.mu.RUnlock()

	return trader.positions[symbol] != nil
}

func (trader *Trader) PositionCount() int {
	if trader == nil {
		return 0
	}

	trader.mu.RLock()
	defer trader.mu.RUnlock()

	return len(trader.positions)
}

func (trader *Trader) Positions() map[string]*broker.Position {
	if trader == nil {
		return nil
	}

	trader.mu.RLock()
	defer trader.mu.RUnlock()

	snapshot := make(map[string]*broker.Position, len(trader.positions))

	for key, value := range trader.positions {
		snapshot[key] = value
	}

	return snapshot
}

func (trader *Trader) Balance() *broker.Balance {
	if trader == nil {
		return nil
	}

	return trader.balance
}
