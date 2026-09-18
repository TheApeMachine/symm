package strategy

import (
	"context"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type Action string

const (
	ActionEnter Action = "enter"
	ActionExit  Action = "exit"
	ActionWait  Action = "wait"
)

/*
Trader runs a compiled nomagique Workspace and listens to its signals, talking to the broker and managing positions.
It now hosts the dynamic JSON workspace and reacts to its emitted signals.
*/
type Trader struct {
	*runtime.System
	workspace *runtime.Workspace
	desk      *broker.Desk
	positions map[string]*broker.Position
}

func NewTrader(ctx context.Context, api *websocket.API, jsonPath string) *Trader {
	trader := &Trader{
		System:    runtime.NewSystem(ctx, "trader"),
		workspace: runtime.NewWorkspace(ctx, "nomagique", jsonPath),
		desk:      broker.NewDesk(ctx, api),
		positions: make(map[string]*broker.Position),
	}
	
	// Start consuming the dynamic JSON graph sink
	go trader.listen()
	
	return trader
}

/*
listen drains the compiled JSON graph sink and executes trading actions.
*/
func (trader *Trader) listen() {
	for signal := range trader.workspace.Sink() {
		// The JSON sink emits dynamic signals.
		// For example, if it's a normalized energy threshold crossing:
		if score, ok := signal.(float64); ok {
			// This threshold should ideally come from the JSON too (e.g. Tanh or Bound)
			// but we handle the final boundary conversion here for now.
			if score > 0.8 {
				trader.OnAction("BTC/USD", ActionEnter) // Hardcoded pair for now
			} else if score < -0.8 {
				trader.OnAction("BTC/USD", ActionExit)
			}
		}
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

