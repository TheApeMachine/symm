package broker

import (
	"context"
	"fmt"
	"iter"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
)

/*
lotBlob is the S3 surface Cache uses for open/closed lot records.
*/
type lotBlob interface {
	WriteAtPrefix(key string, value []byte) (int, error)
	DeletePrefix(prefix string) error
	Stream(ctx context.Context, prefix string) iter.Seq[[]byte]
}

/*
lotRecord is one stored lot: clock fields plus the position object.
Hub ListPrefix and workbench views read this shape.
*/
type lotRecord struct {
	Epoch     uint64    `json:"epoch"`
	Tick      uint64    `json:"tick"`
	Timestamp uint64    `json:"timestamp"`
	Position  *Position `json:"position"`
}

type Cache struct {
	*runtime.System
	mu        sync.RWMutex
	tierKey   string
	store     lotBlob
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
	return newCache(
		ctx, tierKey, transport, price, onSettled,
		store.NewClient(ctx, system.Cfg.Storage.S3.Bucket),
	)
}

func newCache(
	ctx context.Context,
	tierKey string,
	transport network.Client,
	price *Price,
	onSettled func(*Position),
	blob lotBlob,
) *Cache {
	cache := &Cache{
		System:    runtime.NewSystem(ctx, "cache"),
		tierKey:   tierKey,
		store:     blob,
		transport: transport,
		price:     price,
		onSettled: onSettled,
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

	var record lotRecord

	if err := sonic.Unmarshal(raw, &record); err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation, "[broker.Cache.Restore] decode failed", err,
		))

		return nil
	}

	position := record.Position

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
	}

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

	if _, loaded := cache.positions[state][symbol]; loaded {
		cache.mu.Unlock()
		return true
	}

	cache.positions[state][symbol] = position
	cache.mu.Unlock()

	if err := cache.write(state, position); err != nil {
		errnie.Error(err)
	}

	return false
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
	cache.mu.Lock()

	open := cache.positions["open"][symbol]
	closed := cache.positions["closed"][symbol]
	delete(cache.positions["open"], symbol)
	delete(cache.positions["closed"], symbol)
	cache.mu.Unlock()

	for _, position := range []*Position{open, closed} {
		if position == nil || position.ID == "" {
			continue
		}

		_ = cache.store.DeletePrefix(cache.lotKey("open", position.Symbol, position.ID))
		_ = cache.store.DeletePrefix(cache.lotKey("closed", position.Symbol, position.ID))
	}
}

func (cache *Cache) write(state string, position *Position) error {
	if position == nil || position.ID == "" {
		return nil
	}

	return cache.writeKey(cache.lotKey(state, position.Symbol, position.ID), position)
}

func (cache *Cache) move(position *Position) error {
	if position == nil || position.ID == "" {
		return nil
	}

	closed := cache.lotKey("closed", position.Symbol, position.ID)
	open := cache.lotKey("open", position.Symbol, position.ID)

	if err := cache.writeKey(closed, position); err != nil {
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

func (cache *Cache) writeKey(key string, position *Position) error {
	record := lotRecord{
		Epoch:     data.Epoch.Load(),
		Tick:      data.Tick.Load(),
		Timestamp: uint64(time.Now().UnixNano()),
		Position:  position,
	}

	body, err := sonic.Marshal(record)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[broker.Cache.write] failed to encode lot",
			err,
		))
	}

	if _, err = cache.store.WriteAtPrefix(key, body); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[broker.Cache.write] failed to store lot",
			err,
		))
	}

	return nil
}

func (cache *Cache) lotKey(state, symbol, id string) string {
	return fmt.Sprintf("%s/%s/%s/%s", cache.tierKey, state, symbol, id)
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
