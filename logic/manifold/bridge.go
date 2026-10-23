package manifold

import (
	"context"
	"io"
	"slices"

	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

/*
Bridge feeds the resident manifold from workspace ticks and publishes the
latest advance to the dashboard hub. The solver's own advance loop runs
asynchronously; this node marks symbols dirty from trades and ships the
newest reading once the field has stepped.
*/
type Bridge struct {
	*runtime.System
	solver      *Solver
	hub         io.Writer
	lastVersion uint64
}

func NewBridge(ctx context.Context, solver *Solver, hub io.Writer) *Bridge {
	bridge := &Bridge{
		solver: solver,
		hub:    hub,
	}
	bridge.System = runtime.NewSystem(ctx, "manifold", solver)
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
		bridge.solver.Wake(trade.Symbol)
	}

	slices.Sort(symbols)

	savedScope := tick.Scope
	for _, symbol := range symbols {
		tick.Scope = symbol
		bridge.solver.Step(tick)
	}
	tick.Scope = savedScope

	reading := bridge.solver.Reading()
	if reading == nil || bridge.hub == nil {
		return tick
	}

	if reading.Version <= bridge.lastVersion {
		return tick
	}

	// The advance is the shared field. Publish it under the focus when one
	// is selected, so a symbol that did not trade this step still receives
	// the field that stepped.
	scope := reading.Symbol
	if focus := types.Focus(); focus != "" {
		scope = focus
	}

	frame := PublishReading(reading, scope, tick.Epoch, tick.Tick, tick.Timestamp)
	if frame == nil {
		return tick
	}

	io.Copy(bridge.hub, frame)
	bridge.lastVersion = reading.Version
	return tick
}
