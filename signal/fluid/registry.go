package fluid

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/signal/compute"
)

type Registry struct {
	symbols sync.Map
	serial  *compute.SerialPool
}

func NewSyncRegistry() *Registry {
	return &Registry{}
}

func NewRegistry(ctx context.Context) *Registry {
	return &Registry{
		serial: compute.NewSerialPool(
			ctx,
			8192,
			1*time.Minute,
		),
	}
}

func (registry *Registry) Close() {
	if registry.serial != nil {
		registry.serial.Close()
	}
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
