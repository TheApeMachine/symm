package broker

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue/kraken"
)

/*
Cache splits open vs closed lanes because a position stays under open/ through
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
		errnie.Debug("[cache] trades history skipped: nil transport", "tier", tierKey)
		return cache
	}

	type row struct {
		id    string
		trade kraken.HistoryTrade
	}

	rows := make([]row, 0)
	cursor := ""

	for {
		req := kraken.NewTradesHistoryRequest([]string{})
		req.Cursor = cursor

		raw, err := transport.Post(system.Cfg.WebSocket.Endpoints.TradesHistory, req)

		if err != nil {
			errnie.Error(err)
			break
		}

		history := kraken.NewTradesHistoryResult(raw)

		if history == nil {
			break
		}

		for id, trade := range history.Trades {
			rows = append(rows, row{id: id, trade: trade})
		}

		if history.Cursor.Next == "" {
			break
		}

		cursor = history.Cursor.Next
	}

	slices.SortFunc(rows, func(a, b row) int {
		return a.trade.Time.Compare(b.trade.Time)
	})

	active := make(map[string]*Position)

	for _, row := range rows {
		trade := row.trade
		symbol := trade.Pair

		if price != nil && price.normalizer != nil {
			symbol = price.normalizer.Name(symbol)
		}

		if symbol == "" {
			continue
		}

		position := active[symbol]

		if position == nil {
			if trade.Type != "buy" {
				continue
			}

			position = NewPosition(
				cache.Context(),
				symbol,
				tierKey,
				"",
				price,
				cache.transport,
				cache.onSettled,
			)
			position.OpenedAt = trade.Time
			active[symbol] = position
		}

		position.Apply(market.ExecutionData{
			ExecID:      row.id,
			ExecType:    "trade",
			OrderID:     trade.OrderID,
			Symbol:      symbol,
			Side:        trade.Type,
			LastQty:     trade.Volume,
			LastPrice:   trade.Price,
			Cost:        trade.Cost,
			FeeUsdEquiv: trade.Fee,
			OrderType:   trade.OrderType,
			Timestamp:   trade.Time,
			OrderStatus: "filled",
		})

		if position.ClosedAt == nil {
			continue
		}

		cache.closed.Store(position.ID, position)
		delete(active, symbol)
	}

	for symbol, position := range active {
		cache.open.Store(symbol, position)
	}

	return cache
}

/*
Bound drops open positions the wallet no longer holds and aligns qty to
spendable capital. History proposes; Balance confirms — never invents opens.
*/
func (cache *Cache) Bound(balance *market.Balance) {
	if balance == nil {
		return
	}

	// Empty frame must not wipe live positions — that invented "exits" with no trade.
	if len(balance.Assets) == 0 {
		return
	}

	quote := balance.Quote

	if quote == "" {
		quote = system.Cfg.Market.QuoteCurrency
	}

	held := make(map[string]decimal.Decimal)

	for _, asset := range balance.Assets {
		if asset == nil || asset.Symbol == "" || asset.Symbol == quote {
			continue
		}

		if asset.Capital.Sign() <= 0 {
			continue
		}

		held[asset.Symbol] = asset.Capital
	}

	// Still no spendable bases — incomplete snapshot, do not drop.
	if len(held) == 0 {
		return
	}

	for _, position := range cache.Open() {
		if position == nil {
			continue
		}

		// WAITING (order out) and BUSY (exit armed) are not wallet-backed yet
		// or still exiting — Bound must not drop them on a partial snapshot.
		if position.Status() != runtime.READY {
			continue
		}

		base := position.Symbol

		if b, _, ok := strings.Cut(position.Symbol, "/"); ok {
			base = b
		} else if quote != "" && strings.HasSuffix(position.Symbol, quote) {
			base = strings.TrimSuffix(position.Symbol, quote)
		}

		capital, ok := held[base]

		if !ok {
			cache.open.Delete(position.Symbol)
			continue
		}

		// Never raise Qty above fill-derived size — incomplete history + wallet
		// capital invented a cheap entry on a full bag (fake +pnl_pct).
		filled := position.qty()
		if filled == nil || filled.Sign() <= 0 {
			cache.open.Delete(position.Symbol)
			continue
		}

		qty := capital
		if qty.GreaterThan(*filled) {
			qty = *filled
		}

		if position.Qty == nil || !position.Qty.Equal(qty) {
			position.Qty = &qty
		}
	}
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
Add is idempotent on restore — it refuses to overwrite a live open position unless
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
Save writes the full position (orders, fills, mark, pnl, stoploss) to S3.

Positions already under closed/ (or settled and no longer open) update the closed
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
	cache.closed.Store(loaded.ID, loaded)

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
			"[broker.Cache.move] failed to retire open position",
			err,
		))
	}

	return nil
}
