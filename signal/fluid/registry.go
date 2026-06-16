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
	worker  *compute.BatchWorker
}

func NewSyncRegistry() *Registry {
	return &Registry{}
}

func NewRegistry(ctx context.Context) *Registry {
	return &Registry{
		worker: compute.NewBatchWorker(
			ctx,
			8192,
			1*time.Minute,
		),
	}
}

func (registry *Registry) Close() {
	if registry.worker != nil {
		registry.worker.Close()
	}
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

func (registry *Registry) enqueue(symbol string, task func(*FluidSymbol)) {
	if registry == nil || task == nil {
		return
	}

	state := registry.loadSymbol(symbol)

	if state == nil {
		return
	}

	if registry.worker == nil {
		task(state)

		return
	}

	registry.worker.Submit(func() {
		task(state)
	})
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
