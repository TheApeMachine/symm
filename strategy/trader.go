package strategy

import (
	"context"
	"slices"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type Action string

const (
	ActionEnter Action = "enter"
	ActionExit  Action = "exit"
	ActionWait  Action = "wait"
)

/*
LegalActions returns permissible actions based on current inventory holding state.
*/
func LegalActions(holding bool) []Action {
	if !holding {
		return []Action{ActionEnter, ActionWait}
	}

	return []Action{ActionExit, ActionWait}
}

/*
Trader runs a compiled nomagique Workspace and listens to its signals, talking to the broker and managing positions.
It now hosts the dynamic JSON workspace and reacts to its emitted signals.
*/
type Trader struct {
	*runtime.System
	workspace *runtime.Workspace
	desk      *broker.Desk
	positions map[string]*broker.Position
	callbacks []func(cognition.Evaluation)
}

func NewTrader(ctx context.Context, api *websocket.API, jsonPath string) *Trader {
	return NewTraderWithWorkspace(ctx, api, runtime.NewWorkspace(ctx, "nomagique", jsonPath))
}

/*
NewTraderWithWorkspace constructs a Trader with an already compiled nomagique Workspace.
*/
func NewTraderWithWorkspace(ctx context.Context, api *websocket.API, ws *runtime.Workspace) *Trader {
	trader := &Trader{
		System:    runtime.NewSystem(ctx, "trader"),
		workspace: ws,
		desk:      broker.NewDesk(ctx, api),
		positions: make(map[string]*broker.Position),
	}

	// Start consuming the compiled JSON graph sink
	go trader.listen()

	return trader
}

func (trader *Trader) Workspace() *runtime.Workspace {
	return trader.workspace
}

func (trader *Trader) OnEvaluation(cb func(cognition.Evaluation)) {
	trader.callbacks = append(trader.callbacks, cb)
}

/*
listen drains the compiled JSON graph sink and executes trading actions.
*/
func (trader *Trader) listen() {
	for signal := range trader.workspace.Sink() {
		switch s := signal.(type) {
		case cognition.Evaluation:
			if s == nil {
				continue
			}

			winnerClass, _, _, contrast, _, _, isBreak, _, _ := s()
			
			for _, cb := range trader.callbacks {
				cb(s)
			}

			if isBreak {
				continue
			}
			symbol := "BTC/USD"
			legal := LegalActions(trader.Holding(symbol))
			action := Action(string(winnerClass))
			if slices.Contains(legal, action) && contrast > 0 {
				trader.OnAction(symbol, action)
			}
		case Action:
			trader.OnAction("BTC/USD", s)
		case map[string]any:
			if actionStr, ok := s["action"].(string); ok {
				symbol, _ := s["symbol"].(string)
				if symbol == "" {
					symbol = "BTC/USD"
				}
				trader.OnAction(symbol, Action(actionStr))
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

