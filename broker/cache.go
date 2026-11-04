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

	for buf := range cache.store.Stream(cache.Context(), fmt.Sprintf("%s/closed/", cache.tierKey)) {
		position := cache.restore(buf, false)

		if position == nil {
			continue
		}

		cache.keepClosed(position)
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
	return cache.restore(raw, true)
}

func (cache *Cache) restore(raw []byte, reopen bool) *Position {
	if len(raw) == 0 {
		return nil
	}

	position := decodeLot(raw)

	if position == nil || position.Symbol == "" {
		return nil
	}

	if !reopen && position.ClosedAt == nil {
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

	// Open-prefix lots were never Remove'd. A prior settle that set ClosedAt
	// then failed must reopen so mark/settle can finish the exit.
	if reopen {
		position.ClosedAt = nil

		if position.Stoploss != nil {
			position.Stoploss.State = StoplossHealthy
		} else {
			pnlPct, _ := position.PnLPct.Float64()
			position.Stoploss = NewStoploss(pnlPct, cache.price, position.Symbol)
		}
	}

	if len(position.Fills) > 0 {
		position.foldFills()
	}

	return position
}

/*
keepClosed retains the latest settle when several closed ids share a symbol.
*/
func (cache *Cache) keepClosed(position *Position) {
	if position == nil {
		return
	}

	current := cache.positions["closed"][position.Symbol]

	if current != nil && current.ClosedAt != nil && position.ClosedAt != nil &&
		!position.ClosedAt.After(*current.ClosedAt) {
		return
	}

	cache.positions["closed"][position.Symbol] = position
}

/*
decodeLot accepts a bare Position or a blob that nests it under "position".
*/
func decodeLot(raw []byte) *Position {
	var position *Position

	if err := sonic.Unmarshal(raw, &position); err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation, "[broker.Cache.Restore] decode failed", err,
		))

		return nil
	}

	if position != nil && position.Symbol != "" {
		return position
	}

	var wrap struct {
		Position *Position `json:"position"`
	}

	if err := sonic.Unmarshal(raw, &wrap); err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation, "[broker.Cache.Restore] decode failed", err,
		))

		return nil
	}

	return wrap.Position
}

func (cache *Cache) Has(symbol string) bool {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	// Still under open (including ClosedAt claimed while settle Sends) occupies
	// the symbol. Remove is the only path into closed.
	return cache.positions["open"][symbol] != nil
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
