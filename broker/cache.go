package broker

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/network"
)

/*
Cache splits open vs closed lanes because a position stays under open/ through
Closing until exit fills land — desk logic and object keys must match that seam.
*/
type Cache struct {
	*runtime.System
	tierKey string
	store   *store.Client
	stops   *StopHistory
	ledger  *Ledger
	open    *sync.Map
	closed  *sync.Map
}

/*
NewCache wires the tier position lanes. Venue history / private fills admit positions;
the cache does not bootstrap ClosedAt from wall clock or store replay.
*/
func NewCache(
	ctx context.Context,
	tierKey string,
	transport network.Client,
	price *Price,
	onSettled func(*Position),
	client *store.Client,
) *Cache {
	cache := &Cache{
		System:  runtime.NewSystem(ctx, "cache"),
		tierKey: tierKey,
		store:   client,
		stops:   NewStopHistory(tierKey, client),
		ledger:  NewLedger(tierKey, price, transport, onSettled),
		open:    &sync.Map{},
		closed:  &sync.Map{},
	}

	// Restore while INIT: Fold binds orders/fills but leaves opens WAITING so
	// Apply never READY/PnL before Price/Level3 can mark.
	open, closed := cache.ledger.Restore(ctx)

	for _, position := range open {
		cache.Add(position.Symbol, position, true)
	}

	for _, position := range closed {
		cache.archive(position)
	}

	cache.Transition(runtime.WAITING)

	return cache
}

/*
archive is the only closed-lane write. The key is the position id, so one
symbol can keep every flat-to-flat cycle.
*/
func (cache *Cache) archive(position *Position) {
	if cache == nil || position == nil || position.ID == "" {
		return
	}

	cache.closed.Store(position.ID, position)
}

/*
All merges lanes for hub/Hello: Forward needs every archived id, while desk
enter/exit only cares about the latest open key per symbol.
*/
func (cache *Cache) All(symbols ...string) []*Position {
	return append(cache.Open(symbols...), cache.Closed(symbols...)...)
}

/*
Open is filled live positions only — Qty/Entry set. Unfilled submits are not open.
*/
func (cache *Cache) Open(symbols ...string) []*Position {
	if len(symbols) == 0 {
		positions := make([]*Position, 0)

		cache.open.Range(func(_, raw any) bool {
			positions = append(positions, raw.(*Position))
			return true
		})

		return positions
	}

	positions := make([]*Position, 0, len(symbols))

	for _, symbol := range symbols {
		raw, ok := cache.open.Load(symbol)

		if !ok {
			continue
		}

		positions = append(positions, raw.(*Position))
	}

	return positions
}

/*
Closed feeds Unlocked stats and late fill matching — sells may arrive after Remove.
*/
func (cache *Cache) Closed(symbols ...string) []*Position {
	if len(symbols) == 0 {
		positions := make([]*Position, 0)

		cache.closed.Range(func(_, raw any) bool {
			positions = append(positions, raw.(*Position))
			return true
		})

		return positions
	}

	positions := make([]*Position, 0)

	cache.closed.Range(func(_, raw any) bool {
		position := raw.(*Position)

		if slices.Contains(symbols, position.Symbol) {
			positions = append(positions, position)
		}

		return true
	})

	return positions
}

/*
Has is the enter mutex: one symbol may not get a second position until Remove clears
the open key (including positions with ClosedAt set but exit fills in flight).
*/
func (cache *Cache) Has(symbol string) bool {
	// Still under open (including ClosedAt claimed while settle Sends) occupies
	// the symbol. Remove is the only path into closed.
	_, ok := cache.open.Load(symbol)
	return ok
}

/*
Get returns the position by symbol.
*/
func (cache *Cache) Get(symbol string) *Position {
	raw, ok := cache.open.Load(symbol)

	if !ok || raw == nil {
		return nil
	}

	return raw.(*Position)
}

/*
Add is idempotent. The open lane is keyed by symbol and refuses a second live
position unless the incumbent already claimed ClosedAt during settle. The closed
lane is keyed by position id.
*/
func (cache *Cache) Add(symbol string, position *Position, open bool) bool {
	if position == nil {
		return true
	}

	if !open {
		if position.ID == "" {
			return true
		}

		if _, loaded := cache.closed.Load(position.ID); loaded {
			return true
		}

		cache.archive(position)
		return false
	}

	if symbol == "" {
		return true
	}

	if existing, loaded := cache.open.Load(symbol); loaded {
		pos := existing.(*Position)

		if pos == nil || !pos.IsClosed() {
			return true
		}
	}

	cache.open.Store(symbol, position)
	return false
}

/*
Save writes the position store envelope to S3 (epoch/tick/timestamp + nested
position with book). Hub closed-history and DuckDB open/closed views read that
shape; bare Position JSON leaves them empty.

Positions already under closed/ (or settled and no longer open) update the closed
object and retire any open/ orphan; exit fills after Remove must not recreate
an open key with a second mark pass.
*/
func (cache *Cache) Save(position *Position) error {
	position.mu.Lock()
	body, err := encodePositionDoc(position, cache.tierKey)
	symbol := position.Symbol
	id := position.ID
	closedAt := position.ClosedAt

	var stopBody []byte
	var stopBeat string
	if err == nil && position.Stoploss != nil {
		stopBody, err = sonic.Marshal(position.Stoploss)
		stopBeat = position.Stoploss.beat()
	}
	position.mu.Unlock()

	if err != nil {
		return errnie.Error(err)
	}

	key := "open"

	if closedAt != nil {
		key = "closed"
	}

	if cache.store == nil {
		return nil
	}

	if _, err := cache.store.WriteAtPrefix(fmt.Sprintf(
		"%s/%s/%s/%s", cache.tierKey, key, symbol, id,
	), body); err != nil {
		return errnie.Error(err)
	}

	cache.stops.Note(symbol, id, stopBeat, stopBody)

	return nil
}

/*
Remove is the settle handoff: stoploss fired, flat in Apply — open lane retires
and object store moves open/ → closed/.
*/
func (cache *Cache) Remove(symbol string) bool {
	raw, ok := cache.open.LoadAndDelete(symbol)

	if !ok {
		return false
	}

	loaded := raw.(*Position)
	cache.archive(loaded)

	if err := cache.move(loaded); err != nil {
		errnie.Error(err)
	}

	return true
}

/*
Drop is for rejected enters and failed upserts — it must not touch closed/
history or the tier would lose PnL stats.
*/
func (cache *Cache) Drop(symbol string) {
	if symbol == "" {
		return
	}

	cache.open.Delete(symbol)

	// Only the failed open position — closed history must survive a reject.
	if cache.store == nil {
		return
	}

	if err := cache.store.DeletePrefix(fmt.Sprintf("%s/open/%s/", cache.tierKey, symbol)); err != nil {
		errnie.Error(err)
	}
}

/*
move makes S3 match memory after settle; a crash between write and delete must not
leave two authoritative copies of the same position id.
*/
func (cache *Cache) move(position *Position) error {
	if position == nil || position.ID == "" || cache.store == nil {
		return nil
	}

	position.mu.Lock()
	body, err := encodePositionDoc(position, cache.tierKey)
	symbol := position.Symbol
	id := position.ID
	position.mu.Unlock()

	if err != nil {
		return errnie.Error(err)
	}

	closed := fmt.Sprintf("%s/closed/%s/%s", cache.tierKey, symbol, id)
	open := fmt.Sprintf("%s/open/%s/%s", cache.tierKey, symbol, id)

	// WriteSync before DeletePrefix — async WriteAtPrefix could lose the closed
	// object if the process dies between enqueue and the open-key delete.
	if _, err := cache.store.WriteSync(closed, body); err != nil {
		return err
	}

	if err := cache.store.DeletePrefix(open); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[broker.Cache.move] failed to retire open position",
			err,
		))
	}

	return nil
}
