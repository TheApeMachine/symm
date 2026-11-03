package broker

import (
	"context"
	"fmt"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/network"
)

type Cache struct {
	*runtime.System
	mu        sync.RWMutex
	tierKey   string
	store     *store.Client
	transport network.Client
	price     *Price
	onSettled func(*Position)
	positions map[string]map[string]*Position
}

func NewCache(
	ctx context.Context,
	tierKey string,
	transport network.Client,
	price *Price,
	onSettled func(*Position),
) *Cache {
	cache := &Cache{
		System:    runtime.NewSystem(ctx, "cache"),
		tierKey:   tierKey,
		store:     store.NewClient(ctx, "symm"),
		transport: transport,
		price:     price,
		onSettled: onSettled,
		positions: map[string]map[string]*Position{
			"open":   {},
			"closed": {},
		},
	}

	for buf := range cache.store.Stream(cache.Context(), fmt.Sprintf("%s/open/", cache.tierKey)) {
		position := cache.Restore(buf)

		if position == nil {
			continue
		}

		cache.positions["open"][position.Symbol] = position
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

	var position *Position

	if err := sonic.Unmarshal(raw, &position); err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation, "[broker.Cache.Restore] decode failed", err,
		))

		return nil
	}

	if position == nil || position.Symbol == "" {
		return nil
	}

	position.System = runtime.NewSystem(cache.Context(), "position")
	position.transport = cache.transport
	position.price = cache.price
	position.onSettled = cache.onSettled
	position.tierKey = cache.tierKey

	if position.Book == "" {
		position.Book = cache.tierKey
	}

	if position.Stoploss != nil {
		position.Stoploss.rewire()
		position.Stoploss.Observe(cache.price, position.Symbol)
	} else {
		pnlPct, _ := position.PnLPct.Float64()
		position.Stoploss = NewStoploss(pnlPct, cache.price, position.Symbol)
	}

	if len(position.Fills) > 0 {
		position.foldFills()
	}

	return position
}

func (cache *Cache) Has(symbol string) bool {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	position := cache.positions["open"][symbol]

	if position == nil {
		return false
	}

	// ClosedAt set but still parked under open (settle/Exit race) must not
	// block Enter — reclaim into closed.
	if position.ClosedAt != nil {
		delete(cache.positions["open"], symbol)
		cache.positions["closed"][symbol] = position
		return false
	}

	return true
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

	if existing, loaded := cache.positions[state][symbol]; loaded {
		if !(open && existing != nil && existing.ClosedAt != nil) {
			cache.mu.Unlock()
			return true
		}
	}

	cache.positions[state][symbol] = position
	cache.mu.Unlock()

	return false
}

/*
Save writes the full lot (orders, fills, mark, pnl, stoploss) to S3.
force bypasses the open-lot throttle used on mark ticks.
*/
func (cache *Cache) Save(position *Position) {
	body, err := sonic.Marshal(position)

	if err != nil {
		errnie.Error(err)
		return
	}

	if _, err := cache.store.WriteAtPrefix(fmt.Sprintf(
		"%s/open/%s/%s", cache.tierKey, position.Symbol, position.ID,
	), body); err != nil {
		errnie.Error(err)
		return
	}
}

func (cache *Cache) Remove(symbol string) bool {
	cache.mu.Lock()

	loaded, ok := cache.positions["open"][symbol]

	if !ok {
		cache.mu.Unlock()
		return false
	}

	delete(cache.positions["open"], symbol)
	cache.positions["closed"][symbol] = loaded
	cache.mu.Unlock()

	if err := cache.move(loaded); err != nil {
		errnie.Error(err)
	}

	return true
}

func (cache *Cache) Drop(symbol string) {
	if symbol == "" {
		return
	}

	cache.mu.Lock()
	delete(cache.positions["open"], symbol)
	delete(cache.positions["closed"], symbol)
	cache.mu.Unlock()

	// Symbol-scoped prefixes clear every orphan ID under the pair, not only
	// the lot currently held in memory.
	_ = cache.store.DeletePrefix(fmt.Sprintf("%s/open/%s/", cache.tierKey, symbol))
	_ = cache.store.DeletePrefix(fmt.Sprintf("%s/closed/%s/", cache.tierKey, symbol))
}

func (cache *Cache) move(position *Position) error {
	if position == nil || position.ID == "" {
		return nil
	}

	body, err := sonic.Marshal(position)

	if err != nil {
		return errnie.Error(err)
	}

	closed := fmt.Sprintf("%s/closed/%s/%s", cache.tierKey, position.Symbol, position.ID)
	open := fmt.Sprintf("%s/open/%s/%s", cache.tierKey, position.Symbol, position.ID)

	if _, err := cache.store.WriteAtPrefix(closed, body); err != nil {
		return err
	}

	if err := cache.store.DeletePrefix(open); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[broker.Cache.move] failed to retire open lot",
			err,
		))
	}

	return nil
}

func (cache *Cache) list(state string, symbols ...string) []*Position {
	positions := make([]*Position, 0)

	cache.mu.RLock()
	defer cache.mu.RUnlock()

	if len(symbols) == 0 {
		for _, position := range cache.positions[state] {
			if position == nil {
				continue
			}

			if state == "open" && position.ClosedAt != nil {
				continue
			}

			positions = append(positions, position)
		}

		return positions
	}

	for _, symbol := range symbols {
		position := cache.positions[state][symbol]

		if position == nil {
			continue
		}

		if state == "open" && position.ClosedAt != nil {
			continue
		}

		positions = append(positions, position)
	}

	return positions
}
