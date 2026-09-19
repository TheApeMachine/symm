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

func NewDecide(minContrast types.Float) Decide {
	return func(eval cognition.Evaluation) string {
		if eval == nil {
			return "wait"
		}

		minC := 0.0
		if minContrast != nil {
			minC = minContrast(eval)
		}

		winner, _, _, contrast, _, _, isBreak, _, _ := eval()
		if isBreak || contrast <= minC || len(winner) == 0 {
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

func NewGate(initialHolding types.Boolean) Gate {
	holding := false
	if initialHolding != nil {
		holding = initialHolding(nil)
	}

	return func(action string) string {
		if action == "enter" {
			if !holding {
				holding = true
				return "enter"
			}
			return "wait"
		}

		if action == "exit" {
			if holding {
				holding = false
				return "exit"
			}
			return "wait"
		}

		return "wait"
	}
}

/*
Submit maps an action into an execution intent fact.
No structs, pure Value closure.
*/
type Submit types.Value[string, map[string]any]

func NewSubmit(symbol types.String) Submit {
	return func(action string) map[string]any {
		if action != "enter" && action != "exit" {
			return nil
		}

		sym := ""
		if symbol != nil {
			sym = symbol(action)
		}

		return map[string]any{
			"symbol":    sym,
			"action":    action,
			"status":    "SUBMITTED",
			"timestamp": time.Now().UnixNano(),
		}
	}
}
