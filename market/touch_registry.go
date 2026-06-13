package market

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/theapemachine/qpool"
)

var activeTouchRegistry atomic.Pointer[TouchRegistry]

/*
RegisterTouchRegistry installs the process-wide touch registry for signals and story.
*/
func RegisterTouchRegistry(registry *TouchRegistry) {
	activeTouchRegistry.Store(registry)
}

/*
ActiveTouchRegistry returns the installed touch registry.
*/
func ActiveTouchRegistry() *TouchRegistry {
	return activeTouchRegistry.Load()
}

/*
TouchRegistryOnce loads one shared touch registry per process.
*/
type TouchRegistryOnce struct {
	once     sync.Once
	registry *TouchRegistry
	err      error
}

func (loader *TouchRegistryOnce) Load(
	ctx context.Context,
	pool *qpool.Q[any],
) (*TouchRegistry, error) {
	loader.once.Do(func() {
		loader.registry, loader.err = NewTouchRegistry(ctx, pool)
	})

	return loader.registry, loader.err
}
