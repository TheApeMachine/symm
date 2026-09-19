package execution

import (
	"time"

	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Decide resolves a winning action from a cognition.Evaluation.
No structs, pure Value closure.
*/
type Decide types.Value[cognition.Evaluation, string]

func NewDecide() Decide {
	return func(eval cognition.Evaluation) string {
		if eval == nil {
			return "wait"
		}

		winner, _, _, contrast, _, _, isBreak, _, _ := eval()
		if isBreak || contrast <= 0 || len(winner) == 0 {
			return "wait"
		}

		return string(winner)
	}
}

/*
Gate maintains inventory state and filters actions against permissible state transitions.
No structs, pure Value closure.
*/
type Gate types.Value[string, string]

func NewGate() Gate {
	holding := false

	return func(action string) string {
		switch action {
		case "enter":
			if !holding {
				holding = true
				return "enter"
			}
			return "wait"
		case "exit":
			if holding {
				holding = false
				return "exit"
			}
			return "wait"
		default:
			return "wait"
		}
	}
}

/*
Submit maps an action into an execution intent fact.
No structs, pure Value closure.
*/
type Submit types.Value[string, map[string]any]

func NewSubmit() Submit {
	return func(action string) map[string]any {
		if action != "enter" && action != "exit" {
			return nil
		}

		return map[string]any{
			"action":    action,
			"status":    "SUBMITTED",
			"timestamp": time.Now().UnixNano(),
		}
	}
}
