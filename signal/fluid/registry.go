package fluid

import (
	"sync"
	"time"

	"github.com/theapemachine/errnie"
)

type Registry struct {
	symbols sync.Map
}

func NewSyncRegistry() *Registry {
	return &Registry{}
}

func (registry *Registry) Close() {
}

func (registry *Registry) SetInstrumentTickSize(symbol string, priceIncrement float64) {
	if registry == nil || symbol == "" || priceIncrement <= 0 {
		return
	}

	state := registry.loadSymbol(symbol)

	if state == nil {
		return
	}

	state.setInstrumentTickSize(priceIncrement)
}

func (registry *Registry) loadSymbol(symbol string) *FluidSymbol {
	if raw, ok := registry.symbols.Load(symbol); ok {
		return raw.(*FluidSymbol)
	}

	state, err := NewFluidSymbol(symbol)

	if errnie.Error(err) != nil {
		return nil
	}

	raw, _ := registry.symbols.LoadOrStore(symbol, state)

	return raw.(*FluidSymbol)
}

func (registry *Registry) RangeRows(eventAt time.Time, visit func(map[string]any) bool) {
	registry.symbols.Range(func(_, value any) bool {
		state, ok := value.(*FluidSymbol)

		if !ok {
			return true
		}

		row := state.Row()

		if row == nil {
			return true
		}

		return visit(row)
	})
}
