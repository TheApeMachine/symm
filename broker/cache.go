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

	endpoint := system.Cfg.WebSocket.Endpoints.TradesHistory
	rows := make([]row, 0)
	cursor := ""

	for page := 0; page < 50; page++ {
		req := kraken.NewTradesHistoryRequest([]string{})
		req.Cursor = cursor

		raw, err := transport.Post(endpoint, req)

		if err != nil {
			errnie.Debug("[cache] trades history post failed",
				"tier", tierKey, "page", page, "endpoint", endpoint, "err", err,
			)
			errnie.Error(err)
			break
		}

		preview := string(raw)
		if len(preview) > 512 {
			preview = preview[:512]
		}

		errnie.Debug("[cache] trades history post ok",
			"tier", tierKey, "page", page, "bytes", len(raw), "body", preview,
		)

		history := kraken.NewTradesHistoryResult(raw)

		if history == nil {
			errnie.Debug("[cache] trades history parse returned nil",
				"tier", tierKey, "page", page, "bytes", len(raw),
			)
			break
		}

		for id, trade := range history.Trades {
			rows = append(rows, row{id: id, trade: trade})
		}

		errnie.Debug("[cache] trades history page",
			"tier", tierKey, "page", page, "page_n", len(history.Trades), "total_n", len(rows),
			"next", history.Cursor.Next,
		)

		if history.Cursor.Next == "" {
			break
		}

		cursor = history.Cursor.Next
	}

	slices.SortFunc(rows, func(a, b row) int {
		return a.trade.Time.Compare(b.trade.Time)
	})

	errnie.Debug("[cache] trades history rows", "tier", tierKey, "n", len(rows))

	active := make(map[string]*Position)

	for _, row := range rows {
		trade := row.trade
		symbol := trade.Pair

		if price != nil && price.normalizer != nil {
			symbol = price.normalizer.Name(symbol)
		}

		errnie.Debug("[cache] history trade",
			"tier", tierKey,
			"id", row.id,
			"pair", trade.Pair,
			"symbol", symbol,
			"side", trade.Type,
			"vol", trade.Volume,
			"price", trade.Price,
			"time", trade.Time,
		)

		if symbol == "" {
			errnie.Debug("[cache] skip trade: empty symbol", "tier", tierKey, "id", row.id, "pair", trade.Pair)
			continue
		}

		position := active[symbol]

		if position == nil {
			if trade.Type != "buy" {
				errnie.Debug("[cache] skip sell with no active position",
					"tier", tierKey, "symbol", symbol, "id", row.id,
				)
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
			errnie.Debug("[cache] new position from buy",
				"tier", tierKey, "symbol", symbol, "id", position.ID, "trade", row.id,
			)
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

		errnie.Debug("[cache] after apply",
			"tier", tierKey,
			"symbol", symbol,
			"position", position.ID,
			"qty", position.Qty,
			"closed_at", position.ClosedAt,
		)

		if position.ClosedAt == nil {
			continue
		}

		cache.closed.Store(position.ID, position)
		delete(active, symbol)
		errnie.Debug("[cache] position closed",
			"tier", tierKey, "symbol", symbol, "id", position.ID, "trade", row.id,
		)
	}

	for symbol, position := range active {
		if position.PnLPct != nil {
			pnlPct, _ := position.PnLPct.Float64()
			position.Stoploss = NewStoploss(pnlPct)
		}

		cache.open.Store(symbol, position)
		errnie.Debug("[cache] position open",
			"tier", tierKey, "symbol", symbol, "id", position.ID, "qty", position.Qty,
		)
	}

	errnie.Debug("[cache] restore done",
		"tier", tierKey,
		"open", len(cache.Open()),
		"closed", len(cache.Closed()),
	)

	return cache
}

/*
Bound drops open positions the wallet no longer holds and aligns qty to
spendable capital. History proposes; Balance confirms — never invents opens.
*/
func (cache *Cache) Bound(book *market.Balance) {
	if book == nil {
		return
	}

	// Empty frame must not wipe live seats — that invented "exits" with no trade.
	if len(book.Assets) == 0 {
		return
	}

	quote := book.Quote

	if quote == "" {
		quote = system.Cfg.Market.QuoteCurrency
	}

	held := make(map[string]decimal.Decimal)

	for _, asset := range book.Assets {
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
		base := position.Symbol

		if b, _, ok := strings.Cut(position.Symbol, "/"); ok {
			base = b
		} else if quote != "" && strings.HasSuffix(position.Symbol, quote) {
			base = strings.TrimSuffix(position.Symbol, quote)
		}

		capital, ok := held[base]

		if !ok {
			errnie.Debug("[cache] bound drop: not in wallet",
				"tier", cache.tierKey, "symbol", position.Symbol, "base", base, "id", position.ID,
			)
			cache.open.Delete(position.Symbol)
			continue
		}

		if position.Qty == nil || !position.Qty.Equal(capital) {
			qty := capital
			position.Qty = &qty
			errnie.Debug("[cache] bound qty from wallet",
				"tier", cache.tierKey, "symbol", position.Symbol, "qty", qty,
			)
		}

		errnie.Debug("[cache] bound keep",
			"tier", cache.tierKey, "symbol", position.Symbol, "base", base, "qty", position.Qty,
		)
	}

	errnie.Debug("[cache] bound done",
		"tier", cache.tierKey, "open", len(cache.Open()), "wallet_bases", len(held),
	)
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
