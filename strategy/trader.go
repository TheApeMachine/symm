package strategy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/runtime"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
Trader is responsible for talking to the broker and managing positions.
*/
type Trader struct {
	*runtime.System
	desk      *broker.Desk
	balance   *broker.Balance
	price     *broker.Price
	positions map[string]*broker.Position
	decisions []*wire.DecisionT
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
		price:     price,
		positions: make(map[string]*broker.Position),
		decisions: make([]*wire.DecisionT, 0, 50),
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
			trader.recordDecisionLocked(symbol, "enter", 1.0, "precursor trigger")
		}
	case ActionExit:
		position, found := trader.positions[symbol]

		if found && position != nil {
			trader.desk.Exit(position)
			delete(trader.positions, symbol)
			trader.recordDecisionLocked(symbol, "exit", 1.0, "exit trigger")
		}
	}
}

func (trader *Trader) RecordDecision(symbol string, action string, confidence float64, reason string) {
	if trader == nil || symbol == "" {
		return
	}

	trader.mu.Lock()
	defer trader.mu.Unlock()

	trader.recordDecisionLocked(symbol, action, confidence, reason)
}

func (trader *Trader) recordDecisionLocked(symbol string, action string, confidence float64, reason string) {
	decision := &wire.DecisionT{
		Id:         fmt.Sprintf("dec-%s-%d", symbol, time.Now().UnixNano()),
		Symbol:     symbol,
		Action:     action,
		Confidence: confidence,
		Reason:     reason,
		At:         time.Now().UnixNano(),
	}

	if len(trader.decisions) >= 50 {
		trader.decisions = trader.decisions[1:]
	}

	trader.decisions = append(trader.decisions, decision)
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

func (trader *Trader) PositionsWire() *wire.PositionsFrameT {
	if trader == nil {
		return &wire.PositionsFrameT{Rows: []*wire.PositionT{}}
	}

	trader.mu.RLock()
	defer trader.mu.RUnlock()

	rows := make([]*wire.PositionT, 0, len(trader.positions))

	for symbol, position := range trader.positions {
		if position == nil {
			continue
		}

		entryPrice := position.Price()
		volume := position.Volume()
		var mark *decimal.Decimal
		var pnl *decimal.Decimal
		returnPct := 0.0

		if trader.price != nil {
			mark = trader.price.CurrentMark(symbol)
			pnl = trader.price.PnL(symbol, position)
			returnPct = trader.price.ReturnPct(symbol, position)
		}

		entryPriceStr := ""

		if entryPrice != nil {
			entryPriceStr = entryPrice.String()
		}

		volumeStr := ""

		if volume != nil {
			volumeStr = volume.String()
		}

		markStr := entryPriceStr

		if mark != nil {
			markStr = mark.String()
		}

		pnlStr := "0.0000"

		if pnl != nil {
			pnlStr = pnl.String()
		}

		entryAtNs := position.EntryAt.UnixNano()

		if entryAtNs <= 0 {
			entryAtNs = time.Now().UnixNano()
		}

		holding := &wire.HoldingT{
			Status:      "active",
			Symbol:      symbol,
			Asset:       symbol,
			Qty:         volumeStr,
			SellableQty: volumeStr,
			EntryAt:     entryAtNs,
			EntryPrice:  entryPriceStr,
			Mark:        markStr,
			Pnl:         pnlStr,
			ReturnPct:   returnPct,
		}

		decision := &wire.DecisionT{
			Id:         position.OrderID(),
			Symbol:     symbol,
			Action:     "enter",
			Confidence: 1.0,
			At:         entryAtNs,
		}

		rows = append(rows, &wire.PositionT{
			Status:   "active",
			Decision: decision,
			Holding:  holding,
		})
	}

	return &wire.PositionsFrameT{Rows: rows}
}

func (trader *Trader) RecentTrades(limit int) ([]*wire.PositionT, error) {
	wireFrame := trader.PositionsWire()

	if wireFrame == nil {
		return []*wire.PositionT{}, nil
	}

	if limit > 0 && len(wireFrame.Rows) > limit {
		return wireFrame.Rows[:limit], nil
	}

	return wireFrame.Rows, nil
}

func (trader *Trader) DecisionsWire() *wire.StrategyFrameT {
	if trader == nil {
		return &wire.StrategyFrameT{Evaluated: true, Decisions: []*wire.DecisionT{}}
	}

	trader.mu.RLock()
	defer trader.mu.RUnlock()

	decisions := make([]*wire.DecisionT, len(trader.decisions))
	copy(decisions, trader.decisions)

	return &wire.StrategyFrameT{
		Evaluated: true,
		Outcome:   "active",
		Decisions: decisions,
	}
}
