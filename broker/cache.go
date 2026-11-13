package broker

import (
	"context"
	"fmt"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue/kraken"
)

/*
Cache splits open vs closed lanes because a lot stays under open/ through
Closing until exit fills land — desk logic and object keys must match that seam.
*/
type Cache struct {
	*runtime.System
	tierKey   string
	store     *store.Client
	transport network.Client
	price     *Price
	onSettled func(*Position)
	open      *sync.Map
	closed    *sync.Map
}

/*
NewCache wires the tier lot lanes. Venue history / private fills admit lots;
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
		System:    runtime.NewSystem(ctx, "cache"),
		tierKey:   tierKey,
		store:     client,
		transport: transport,
		price:     price,
		onSettled: onSettled,
		open:      &sync.Map{},
		closed:    &sync.Map{},
	}

	if transport == nil {
		return cache
	}

	raw, err := transport.Post(
		system.Cfg.WebSocket.Endpoints.TradesHistory,
		kraken.NewTradesHistoryRequest([]string{}),
	)

	if err != nil {
		errnie.Error(err)
		return cache
	}

	history := kraken.NewTradesHistoryResult(raw)

	if history == nil {
		return cache
	}

	for _, trade := range history.Trades {
		position := NewPosition(
			cache.Context(),
			trade.Pair,
			tierKey,
			"",
			price,
			cache.transport,
			cache.onSettled,
		)

		position.Apply(market.ExecutionData{
			ExecID:      trade.OrderID,
			ExecType:    "trade",
			OrderID:     trade.OrderID,
			Symbol:      trade.Pair,
			Side:        trade.Type,
			LastQty:     trade.Volume,
			LastPrice:   trade.Price,
			Cost:        trade.Cost,
			FeeUsdEquiv: trade.Fee,
			OrderType:   trade.OrderType,
			Timestamp:   trade.Time,
			OrderStatus: "filled",
		})

		position.PnLPct = price.PnLPct(
			position.Symbol,
			position.Qty,
			position.Entry,
			position.Exit,
		)

		pnlPct, _ := position.PnLPct.Float64()
		position.Stoploss = NewStoploss(pnlPct)

		if position.ClosedAt == nil {
			cache.open.Store(position.Symbol, position)
		} else {
			cache.closed.Store(position.Symbol, position)
		}
	}

	return cache
}

/*
All merges lanes for hub/Hello: Forward needs every archived id, while desk
enter/exit only cares about the latest open key per symbol.
*/
func (cache *Cache) All(symbols ...string) []*Position {
	return append(cache.Open(symbols...), cache.Closed(symbols...)...)
}

/*
Open is filled live lots only — Qty/Entry set. Unfilled submits are not open.
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

	positions := make([]*Position, 0, len(symbols))

	for _, symbol := range symbols {
		raw, ok := cache.closed.Load(symbol)

		if !ok {
			continue
		}

		positions = append(positions, raw.(*Position))
	}

	return positions
}

/*
Has is the enter mutex: one symbol may not get a second lot until Remove clears
the open key (including lots with ClosedAt set but exit fills in flight).
*/
func (cache *Cache) Has(symbol string) bool {
	// Still under open (including ClosedAt claimed while settle Sends) occupies
	// the symbol. Remove is the only path into closed.
	_, ok := cache.open.Load(symbol)
	return ok
}

/*
Add is idempotent on restore — it refuses to overwrite a live open lot unless
the incumbent already claimed ClosedAt during settle.
*/
func (cache *Cache) Add(symbol string, position *Position, open bool) bool {
	if symbol == "" || position == nil {
		return true
	}

	lane := cache.closed

	if open {
		lane = cache.open
	}

	if existing, loaded := lane.Load(symbol); loaded {
		pos := existing.(*Position)

		if !(open && pos != nil && pos.ClosedAt != nil) {
			return true
		}
	}

	lane.Store(symbol, position)
	return false
}

/*
Save writes the full lot (orders, fills, mark, pnl, stoploss) to S3.

Lots already under closed/ (or settled and no longer open) update the closed
object and retire any open/ orphan — exit fills after Remove must not recreate
an open key with a second mark pass.
*/
func (cache *Cache) Save(position *Position) error {
	position.mu.Lock()
	body, err := sonic.Marshal(position)
	symbol := position.Symbol
	id := position.ID
	closedAt := position.ClosedAt
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
	cache.closed.Store(symbol, loaded)

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

	// Only the failed open lot — closed history must survive a reject.
	if cache.store == nil {
		return
	}

	if err := cache.store.DeletePrefix(fmt.Sprintf("%s/open/%s/", cache.tierKey, symbol)); err != nil {
		errnie.Error(err)
	}
}

/*
move makes S3 match memory after settle; a crash between write and delete must not
leave two authoritative copies of the same lot id.
*/
func (cache *Cache) move(position *Position) error {
	if position == nil || position.ID == "" || cache.store == nil {
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
