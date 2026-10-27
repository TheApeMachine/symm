package broker

import (
	"context"
	"iter"
	"slices"
	"strings"
	"sync"

	"github.com/theapemachine/symm/nomagique/runtime"
)

type Cache struct {
	*runtime.System
	open   *sync.Map
	closed *sync.Map
}

func NewCache(ctx context.Context) *Cache {
	return &Cache{
		System: runtime.NewSystem(ctx, "cache"),
		open:   &sync.Map{},
		closed: &sync.Map{},
	}
}

func (cache *Cache) AllPositions() []*Position {
	positions := make([]*Position, 0)

	cache.open.Range(func(key, value any) bool {
		positions = append(positions, value.(*Position))
		return true
	})

	cache.closed.Range(func(key, value any) bool {
		positions = append(positions, value.(*Position))
		return true
	})

	return positions
}

/*
OpenPositions returns all positions in the cache, ordered by symbol so the
published frame is deterministic: without the sort, sync.Map's random
range order reshuffles the frontend's rows every tick.
*/
func (cache *Cache) OpenPositions() []*Position {
	positions := make([]*Position, 0)

	cache.open.Range(func(key, value any) bool {
		positions = append(positions, value.(*Position))
		return true
	})

	return positions
}

func (cache *Cache) ClosedPositions() []*Position {
	positions := make([]*Position, 0)

	cache.closed.Range(func(key, value any) bool {
		positions = append(positions, value.(*Position))
		return true
	})

	return positions
}

/*
Stream the positions in the cache.
*/
func (cache *Cache) Stream(prefix string) iter.Seq[*Position] {
	return func(yield func(*Position) bool) {
		cache.open.Range(func(key, value any) bool {
			if !yield(value.(*Position)) {
				return false
			}

			return true
		})
	}
}

/*
Get a position from the cache.
*/
func (cache *Cache) Get(symbol string, open bool) *Position {
	if open {
		found, ok := cache.open.Load(symbol)

		if !ok {
			return nil
		}

		return found.(*Position)
	}

	found, ok := cache.closed.Load(symbol)

	if !ok {
		return nil
	}

	return found.(*Position)
}

/*
Restore re-admits a position from history. A position without a symbol is
not addressable — it can never be matched by an update or closed — so it
never enters the cache.
*/
func (cache *Cache) Restore(position *Position) {
	if position == nil || position.Symbol == "" {
		return
	}

	if position.ClosedAt != nil && !position.ClosedAt.IsZero() {
		cache.closed.Store(position.Symbol, position)
	} else {
		cache.open.Store(position.Symbol, position)
	}
}

/*
Count reports how many positions sit on the given side.
*/
func (cache *Cache) Count(open bool) int {
	side := cache.closed

	if open {
		side = cache.open
	}

	count := 0

	side.Range(func(_, _ any) bool {
		count++
		return true
	})

	return count
}

func (cache *Cache) Has(symbol string) bool {
	_, ok := cache.open.Load(symbol)
	return ok
}

func (cache *Cache) Add(symbol string, position *Position) bool {
	if symbol == "" {
		return true
	}

	_, ok := cache.open.LoadOrStore(symbol, position)
	return ok
}

/*
Closed returns positions on the closed side only, in symbol order.
*/
func (cache *Cache) Closed() []*Position {
	positions := make([]*Position, 0)

	cache.closed.Range(func(key, value any) bool {
		positions = append(positions, value.(*Position))
		return true
	})

	slices.SortFunc(positions, func(a, b *Position) int {
		return strings.Compare(a.Symbol, b.Symbol)
	})

	return positions
}

func (cache *Cache) Remove(symbol string) bool {
	loaded, ok := cache.open.LoadAndDelete(symbol)

	if !ok {
		return false
	}

	position := loaded.(*Position)
	cache.closed.Store(symbol, position)
	return true
}

/*
Drop retires a position from both sides of the cache. Remove closes it and
keeps it referenceable; Drop is the separate retirement once its outcome
has been settled.
*/
func (cache *Cache) Drop(symbol string) {
	cache.open.Delete(symbol)
	cache.closed.Delete(symbol)
}
