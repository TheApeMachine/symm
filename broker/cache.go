package broker

import (
	"context"
	"fmt"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/system"
)

type Cache struct {
	*runtime.System
	mu        sync.RWMutex
	tierKey   string
	store     *store.Client
	transport Transport
	positions map[string]map[string]*Position
}

func NewCache(
	ctx context.Context, tierKey string, transport Transport,
) *Cache {
	cache := &Cache{
		System:    runtime.NewSystem(ctx, "cache"),
		tierKey:   tierKey,
		store:     store.NewClient(ctx, system.Cfg.Storage.S3.Bucket),
		transport: transport,
		positions: make(map[string]map[string]*Position),
	}

	for _, state := range []string{"open", "closed"} {
		cache.positions[state] = make(map[string]*Position)

		for raw := range cache.store.Stream(
			cache.Context(), fmt.Sprintf("%s/%s", cache.tierKey, state),
		) {
			position := cache.Restore(raw)

			if position == nil {
				continue
			}

			cache.positions[state][position.Symbol] = position
		}
	}

	return cache
}

func (cache *Cache) All(symbols ...string) []*Position {
	return append(cache.Open(symbols...), cache.Closed(symbols...)...)
}

func (cache *Cache) Open(symbols ...string) []*Position {
	return cache.list("open", symbols...)
}

func (cache *Cache) Closed(symbols ...string) []*Position {
	return cache.list("closed", symbols...)
}

func (cache *Cache) Restore(raw []byte) *Position {
	if len(raw) == 0 {
		return nil
	}

	position := &Position{}

	if err := sonic.Unmarshal(raw, position); err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation, "[broker.Cache.Restore] decode failed", err,
		))

		return nil
	}

	if position.Symbol == "" {
		return nil
	}

	position.System = runtime.NewSystem(cache.Context(), "position")
	position.transport = cache.transport

	return position
}

func (cache *Cache) Has(symbol string) bool {
	cache.mu.RLock()
	defer cache.mu.RUnlock()

	_, ok := cache.positions["open"][symbol]
	return ok
}

func (cache *Cache) Add(symbol string, position *Position, open bool) bool {
	if symbol == "" || position == nil {
		return true
	}

	state := "closed"

	if open {
		state = "open"
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()

	if _, loaded := cache.positions[state][symbol]; loaded {
		return true
	}

	cache.positions[state][symbol] = position
	return false
}

func (cache *Cache) Remove(symbol string) bool {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	loaded, ok := cache.positions["open"][symbol]

	if !ok {
		return false
	}

	delete(cache.positions["open"], symbol)
	cache.positions["closed"][symbol] = loaded
	return true
}

func (cache *Cache) Drop(symbol string) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	delete(cache.positions["open"], symbol)
	delete(cache.positions["closed"], symbol)
}

func (cache *Cache) list(state string, symbols ...string) []*Position {
	positions := make([]*Position, 0)

	cache.mu.RLock()
	defer cache.mu.RUnlock()

	if len(symbols) == 0 {
		for _, position := range cache.positions[state] {
			if position != nil {
				positions = append(positions, position)
			}
		}

		return positions
	}

	for _, symbol := range symbols {
		if position := cache.positions[state][symbol]; position != nil {
			positions = append(positions, position)
		}
	}

	return positions
}
