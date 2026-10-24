package resonance

import (
	"context"
	"io"
	"slices"

	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/ui"
)

/*
Bridge runs one predictive-coder Step per symbol on each aggregate workspace
tick and publishes each reading to the dashboard hub. Workspace nodes only
return the shared tick; resonance readings are separate Values.
*/
type Bridge struct {
	*runtime.System
	solver *Solver
}

func NewBridge(ctx context.Context, solver *Solver) *Bridge {
	bridge := &Bridge{
		solver: solver,
	}
	bridge.System = runtime.NewSystem(ctx, "resonance", solver)
	bridge.Transition(runtime.READY)
	return bridge
}

func (bridge *Bridge) Step(tick *data.Value) *data.Value {
	if tick == nil || bridge.solver == nil {
		return tick
	}

	if bridge.solver.Status() != runtime.READY {
		return tick
	}

	var trades []kraken.TradeData
	tick.Lookup(&trades, "spot:trade")

	if len(trades) == 0 {
		return tick
	}

	symbols := make([]string, 0, len(trades))
	seen := map[string]struct{}{}

	for _, trade := range trades {
		if trade.Symbol == "" {
			continue
		}

		if _, ok := seen[trade.Symbol]; ok {
			continue
		}

		seen[trade.Symbol] = struct{}{}
		symbols = append(symbols, trade.Symbol)
	}

	slices.Sort(symbols)

	savedScope := tick.Scope

	for _, symbol := range symbols {
		tick.Scope = symbol

		reading, err := bridge.solver.Step(tick)
		if err != nil {
			bridge.Error(err)
			continue
		}

		if reading != nil {
			io.Copy(ui.UI, reading)
		}
	}

	tick.Scope = savedScope
	return tick
}
