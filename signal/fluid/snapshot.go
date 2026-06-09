package fluid

import (
	"fmt"
	"time"
)

/*
publishFieldSnapshot ships the current universe field rows to the ui broadcast.
Each symbol row is whatever FluidSymbol.Row() returns; the hub forwards it unchanged.
*/
func (system *System) publishFieldSnapshot(eventAt time.Time) error {
	symbols := make([]map[string]any, 0, 64)

	system.symbols.Range(func(key, value any) bool {
		state, ok := value.(*FluidSymbol)

		if !ok {
			return true
		}

		row := state.Row()

		if row == nil {
			return true
		}

		symbols = append(symbols, row)

		return true
	})

	if len(symbols) == 0 {
		return nil
	}

	if eventAt.IsZero() {
		return fmt.Errorf("fluid: field snapshot event time is zero")
	}

	return system.bus.Send("ui", "field_snapshot", map[string]any{
		"type":         "fluid",
		"ts":           eventAt.UTC().Format(time.RFC3339Nano),
		"symbol_count": len(symbols),
		"symbols":      symbols,
	})
}
