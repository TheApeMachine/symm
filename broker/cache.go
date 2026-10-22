package broker

import (
	"context"
	"iter"
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

/*
All returns all positions in the cache.
If you do not provide a boolean, it will return all open & closed positions.
If you provide a false boolean, it will return all open & closed positions.
If you provide a true boolean, it will return all open positions.
*/
func (cache *Cache) All(open ...bool) []*Position {
	positions := make([]*Position, 0)

	cache.open.Range(func(key, value any) bool {
		positions = append(positions, value.(*Position))
		return true
	})

	if len(open) == 0 || !open[0] {
		cache.closed.Range(func(key, value any) bool {
			positions = append(positions, value.(*Position))
			return true
		})
	}

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

func (cache *Cache) Restore(position *Position) {
	if position.ClosedAt != nil && !position.ClosedAt.IsZero() {
		cache.closed.Store(position.Symbol, position)
	} else {
		cache.open.Store(position.Symbol, position)
	}
}

/*
Drop retires one lot from the closed side, but only if it is still the given
lot (id match). Closed lots stay referenceable from Remove until settlement
finishes with them; Drop is that finish.
*/
func (cache *Cache) Drop(symbol, id string) bool {
	loaded, ok := cache.closed.Load(symbol)

	if !ok {
		return false
	}

	if position := loaded.(*Position); position.ID != id {
		return false
	}

	cache.closed.Delete(symbol)
	return true
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
	_, ok := cache.open.LoadOrStore(symbol, position)
	return ok
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
