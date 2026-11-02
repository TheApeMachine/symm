package broker

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker/stoploss"
	"github.com/theapemachine/symm/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
)

const (
	cachePaper   = "paper"
	cacheReal    = "real"
	cacheVirtual = "virtual"
	cacheShadow  = "shadow"
)

/*
Desk owns open positions across concurrent books plus a learning-shadow
cache. Transports, balances, and caches are all keyed the same way.
*/
type Desk struct {
	*runtime.System
	transports     map[string]Transport
	balances       map[string]*Balance
	caches         map[string]*Cache
	price          *Price
	history        *History
	adaptive       *stoploss.Rails
	virtualArchive *sync.Map
	pending        *sync.Map
}

func NewDesk(
	ctx context.Context,
	transports map[string]Transport,
	price *Price,
	balances map[string]*Balance,
) *Desk {
	if transports == nil {
		transports = map[string]Transport{}
	}

	if balances == nil {
		balances = map[string]*Balance{}
	}

	desk := &Desk{
		System:     runtime.NewSystem(ctx, "desk"),
		transports: transports,
		balances:   balances,
		caches: map[string]*Cache{
			cachePaper:   NewCache(ctx),
			cacheReal:    NewCache(ctx),
			cacheVirtual: NewCache(ctx),
			cacheShadow:  NewCache(ctx),
		},
		price:          price,
		adaptive:       stoploss.NewRails(),
		virtualArchive: &sync.Map{},
		pending:        &sync.Map{},
	}

	desk.System = runtime.NewSystem(ctx, "desk", desk)
	desk.adaptive.AttachStore(ctx, store.NewClient(ctx, system.Cfg.Storage.S3.Bucket))

	desk.history = NewHistory(
		ctx,
		price,
		desk.caches,
		desk.transports,
		desk.onPositionSettled,
		desk.adaptive,
	)

	desk.Transition(runtime.READY)

	return desk
}

/*
Hello is a callback for the ui.Hub that fires when a new WebSocket
connection is established, and we want to update the client with
some initial data.
*/
func (desk *Desk) Hello() {
	desk.publishPositions("snapshot", desk.allPositions()...)
	desk.publishBalances()
}

func (desk *Desk) publishBalances() {
	if ui.UI == nil || !ui.UI.Active() {
		return
	}

	frame := map[string]any{}

	for _, key := range []string{cachePaper, cacheReal} {
		balance := desk.balances[key]
		if balance == nil {
			continue
		}

		if reading := balance.Reading(); reading != nil {
			frame[key] = reading
		}
	}

	if len(frame) == 0 {
		return
	}

	ui.UI.WriteJSON(map[string]any{
		"balances": frame,
	})

	// Keep legacy single-balance frame for older clients.
	if paper := desk.balances[cachePaper]; paper != nil {
		paper.Publish()
	} else if live := desk.balances[cacheReal]; live != nil {
		live.Publish()
	}
}

/*
publishPositions fans open lots to the dashboard as plain JSON.
*/
func (desk *Desk) publishPositions(mode string, lots ...*Position) {
	if ui.UI == nil || !ui.UI.Active() {
		return
	}

	book := make(map[string]any, len(lots))

	for _, position := range lots {
		if position == nil {
			continue
		}

		scope := position.ID

		if scope == "" {
			scope = position.Symbol
			ledger := position.Ledger()

			if ledger != LedgerPaper {
				scope = ledger.HistoryKey() + ":" + scope
			}
		}

		if scope == "" {
			continue
		}

		book[scope] = position
	}

	ui.UI.WriteJSON(map[string]any{
		"positions":      book,
		"positions_mode": mode,
	})
}

/*
OpenPositions returns the open positions on the venue/virtual books.
*/
func (desk *Desk) OpenPositions() []*Position {
	out := make([]*Position, 0)

	for _, cache := range desk.bookCaches() {
		for _, position := range cache.OpenPositions() {
			if position == nil || position.Symbol == "" {
				continue
			}

			out = append(out, position)
		}
	}

	return out
}

/*
Virtuals returns the virtual positions in the desk.
*/
func (desk *Desk) Virtuals() []*Position {
	return desk.caches[cacheVirtual].AllPositions()
}

/*
Positions returns every open lot for a symbol across the trading books.
*/
func (desk *Desk) Positions(symbol string) []*Position {
	if symbol == "" {
		return nil
	}

	out := make([]*Position, 0, 3)

	for _, cache := range desk.bookCaches() {
		if position := cache.Get(symbol, true); position != nil {
			out = append(out, position)
		}
	}

	return out
}

/*
Position returns one open lot for the symbol. Prefer venue books, then virtual.
*/
func (desk *Desk) Position(symbol string) *Position {
	for _, key := range []string{cachePaper, cacheReal, cacheVirtual} {
		if position := desk.caches[key].Get(symbol, true); position != nil {
			return position
		}
	}

	return nil
}

/*
PositionOn returns the open lot on one book.
*/
func (desk *Desk) PositionOn(symbol string, ledger Ledger) *Position {
	return desk.cache(ledger.String()).Get(symbol, true)
}

/*
ClosedVirtuals returns virtual lots settled this tick and still on the closed
side of the live cache, awaiting promotion scoring.
*/
func (desk *Desk) ClosedVirtuals() []*Position {
	return desk.caches[cacheVirtual].Closed()
}

func (desk *Desk) onPositionSettled(position *Position) {
	if position == nil {
		return
	}

	book := position.Ledger()
	desk.pending.Store(book.PendingKey(position.Symbol), position)

	desk.cache(book.String()).Remove(position.Symbol)

	if desk.history != nil {
		closed := book.HistoryKey() + "/closed/" + position.Symbol + "/" + position.ID
		open := book.HistoryKey() + "/open/" + position.Symbol + "/" + position.ID

		if err := desk.history.Move(closed, position, open); err != nil {
			errnie.Error(err)
		}
	}

	// Learning shadow keeps receiving marks after the active structural settle.
	if position.ShadowLive() {
		position.onShadowDone = desk.onShadowDone
		desk.caches[cacheShadow].Add(position.ID, position, true)
		desk.publishPositions("update", position)
		return
	}

	if position.Learning.ExitAt != nil {
		desk.persistShadow(position)
	}

	desk.publishPositions("update", position)
}

/*
Refresh reprices open lots and learning shadows from the book mid.
*/
func (desk *Desk) Refresh(symbols []string) {
	for _, symbol := range symbols {
		if symbol == "" {
			continue
		}

		desk.markSymbol(symbol, desk.price.CurrentMark(symbol))
	}
}

/*
Update marks open lots and learning shadows from trade prints.
*/
func (desk *Desk) Update(trades []kraken.TradeData) {
	desk.price.Tape.Note(trades)

	for _, trade := range trades {
		desk.markSymbol(trade.Symbol, &trade.Price)
	}
}

/*
markSymbol advances every open lot and learning shadow on one symbol, then
publishes once.
*/
func (desk *Desk) markSymbol(symbol string, mark *decimal.Decimal) {
	if symbol == "" || mark == nil {
		return
	}

	updated := make([]*Position, 0)

	for _, position := range desk.Positions(symbol) {
		position.Update(mark)

		if !position.IsClosed() {
			desk.persistOpen(position)
		}

		updated = append(updated, position)
	}

	for _, position := range desk.caches[cacheShadow].OpenPositions() {
		if position == nil || position.Symbol != symbol {
			continue
		}

		if position.UpdateShadow(mark) {
			desk.onShadowDone(position)
		}

		updated = append(updated, position)
	}

	if len(updated) > 0 {
		desk.publishPositions("update", updated...)
	}
}

func (desk *Desk) onShadowDone(position *Position) {
	if position == nil {
		return
	}

	desk.caches[cacheShadow].Drop(position.ID)
	desk.persistShadow(position)
}

func (desk *Desk) persistOpen(position *Position) {
	if desk.history == nil || position == nil || position.IsClosed() {
		return
	}

	book := position.Ledger()

	if err := desk.history.Store(
		book.HistoryKey()+"/open/"+position.Symbol+"/"+position.ID,
		position,
	); err != nil {
		errnie.Error(err)
	}
}

func (desk *Desk) persistShadow(position *Position) {
	if desk.history == nil || position == nil {
		return
	}

	// Rewrite the closed lot once the learning shadow finishes so history
	// carries exit_at — the mid-settle Store still has learning.active.
	book := position.Ledger()

	if err := desk.history.Store(
		book.HistoryKey()+"/closed/"+position.Symbol+"/"+position.ID,
		position,
	); err != nil {
		errnie.Error(err)
	}
}

/*
Has reports whether the desk holds an open position on any trading book.
*/
func (desk *Desk) Has(symbol string) bool {
	for _, cache := range desk.bookCaches() {
		if cache.Has(symbol) {
			return true
		}
	}

	return false
}

/*
CanSize reports whether the book and budget can allocate a venue-valid lot.
*/
func (desk *Desk) CanSize(symbol string) bool {
	if symbol == "" {
		return false
	}

	qty, _ := desk.price.Allocate(symbol)
	return qty != nil && qty.Sign() > 0
}

/*
BookStale reports whether a symbol's book is stale or absent in the price book manager.
*/
func (desk *Desk) BookStale(symbol string) bool {
	return desk.price.Books.StaleSymbol(symbol)
}

/*
Apply an execution to the desk.
*/
func (desk *Desk) Apply(executions *kraken.Execution) {
	for _, execution := range executions.Data {
		for _, cache := range desk.bookCaches() {
			position := cache.Get(execution.Symbol, true)

			if position == nil {
				continue
			}

			position.Apply(execution)
		}
	}
}

/*
Enter opens a lot on one book. Slot checks are per-book: virtual, paper, and
real may all hold the same symbol at once.
*/
func (desk *Desk) Enter(
	symbol string, path string, ledger Ledger, pathFound bool,
) error {
	if ledger == "" {
		ledger = LedgerPaper
	}

	cache := desk.cache(ledger.String())

	if cache.Has(symbol) {
		why := ledger.HistoryKey() + " already exists"
		desk.auditEntryBlocked(symbol, "slot", why, ledger)

		return &EntryDenial{Gate: "slot", Reason: why}
	}

	var (
		qty *decimal.Decimal
		why string
	)

	switch ledger {
	case LedgerVirtual:
		qty, why = desk.price.AllocateVirtual(symbol)
	case LedgerReal:
		qty, why = desk.price.AllocateFrom(desk.balances[cacheReal], symbol, true)
	default:
		qty, why = desk.price.AllocateFrom(desk.balances[cachePaper], symbol, true)
	}

	if qty == nil || qty.Sign() <= 0 {
		errnie.Warn("[desk.Enter] failed to allocate quantity", "symbol", symbol, "why", why)
		desk.auditEntryBlocked(symbol, "allocate", why, ledger)

		return &EntryDenial{Gate: "allocate", Reason: why}
	}

	bypass := system.Cfg.Learning.ResonanceBypass

	if !(ledger.Virtual() && bypass) {
		if phys := CheckEntryPhysical(desk.price, symbol); !phys.Allowed {
			desk.auditEntryBlocked(symbol, "physical", phys.Reason, ledger)

			return &EntryDenial{Gate: "physical", Reason: phys.Reason}
		}
	}

	position := NewPositionWithRails(
		desk.Context(),
		symbol,
		path,
		pathFound,
		ledger,
		desk.price,
		desk.transports[ledger.String()],
		desk.onPositionSettled,
		desk.adaptive,
	)

	cache.Add(position.Symbol, position, true)

	if err := position.Enter(path, qty); err != nil {
		cache.Drop(position.Symbol)
		desk.auditEntryBlocked(symbol, "send", err.Error(), ledger)

		return &EntryDenial{Gate: "send", Reason: err.Error()}
	}

	if err := desk.history.Store(
		ledger.HistoryKey()+"/open/"+symbol+"/"+position.ID,
		position,
	); err != nil {
		cache.Drop(position.Symbol)

		return errnie.Error(errnie.Err(
			errnie.IO,
			"[desk.Enter] failed to store position",
			err,
		))
	}

	desk.publishPositions("update", position)

	return nil
}

func (desk *Desk) auditEntryBlocked(symbol, gate, reason string, ledger Ledger) {
	audit.Record.Write(map[string]any{
		"node":    "entry",
		"phase":   "blocked",
		"symbol":  symbol,
		"gate":    gate,
		"reason":  reason,
		"book":    ledger.String(),
		"virtual": ledger.Virtual(),
	})
}

/*
Exit a position on the requested book.
*/
func (desk *Desk) Exit(
	symbol string, path string, ledger Ledger,
) error {
	caches := []*Cache{desk.cache(ledger.String())}

	// Fall back across books if the caller only knew the symbol.
	if ledger == "" {
		caches = desk.bookCaches()
	}

	for _, cache := range caches {
		position := cache.Get(symbol, true)

		if position == nil {
			continue
		}

		already := position.IsClosed()

		if err := position.Exit(path); err != nil {
			if errors.Is(err, ErrExitInProgress) {
				return nil
			}

			return err
		}

		if !already && position.IsClosed() {
			desk.onPositionSettled(position)
		}

		return nil
	}

	return nil
}

func (desk *Desk) TakePending(key string) (*Position, bool) {
	if key == "" {
		return nil, false
	}

	val, ok := desk.pending.LoadAndDelete(key)

	if !ok {
		return nil, false
	}

	position, ok := val.(*Position)
	return position, ok && position != nil
}

/*
PendingPut restores a settle that was taken before an exit cut was ready.
*/
func (desk *Desk) PendingPut(key string, position *Position) {
	if key == "" || position == nil {
		return
	}

	desk.pending.Store(key, position)
}

/*
Pending returns pending settle keys (book:symbol).
*/
func (desk *Desk) Pending() []string {
	out := make([]string, 0)

	desk.pending.Range(func(key, value any) bool {
		if symbol, ok := key.(string); ok && symbol != "" {
			out = append(out, symbol)
		}

		return true
	})

	slices.Sort(out)
	return out
}

/*
ParsePendingKey splits a book:symbol pending key.
*/
func ParsePendingKey(key string) (Ledger, string) {
	book, symbol, ok := strings.Cut(key, ":")
	if !ok {
		return LedgerPaper, key
	}

	return ParseLedger(book, book == string(LedgerVirtual)), symbol
}

func (desk *Desk) cache(key string) *Cache {
	if cache := desk.caches[key]; cache != nil {
		return cache
	}

	return desk.caches[cachePaper]
}

func (desk *Desk) bookCaches() []*Cache {
	out := make([]*Cache, 0, 3)

	for _, key := range []string{cachePaper, cacheReal, cacheVirtual} {
		if cache := desk.caches[key]; cache != nil {
			out = append(out, cache)
		}
	}

	return out
}

func (desk *Desk) allPositions() []*Position {
	out := make([]*Position, 0)

	for _, key := range []string{cachePaper, cacheReal, cacheVirtual, cacheShadow} {
		if cache := desk.caches[key]; cache != nil {
			out = append(out, cache.AllPositions()...)
		}
	}

	return out
}
