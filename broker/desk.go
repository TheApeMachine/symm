package broker

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
)

/*
Desk owns open positions across three concurrent books: virtual (learning),
paper (validated paths), and real (live wallet when confidence is armed).
*/
type Desk struct {
	*runtime.System
	paperTransport Transport
	liveTransport  Transport
	price          *Price
	paperBalance   *Balance
	liveBalance    *Balance
	history        *History
	papers         *Cache
	reals          *Cache
	virtuals       *Cache
	shadows        *sync.Map // id → *Position learning ghosts after official settle
	adaptive       *AdaptiveRails
	virtualArchive *sync.Map
	pending        *sync.Map
}

type DeskTransports struct {
	Paper Transport
	Live  Transport
}

type DeskBalances struct {
	Paper *Balance
	Live  *Balance
}

func NewDesk(
	ctx context.Context,
	transports DeskTransports,
	price *Price,
	balances DeskBalances,
) *Desk {
	paperTransport := transports.Paper
	if paperTransport == nil {
		paperTransport = transports.Live
	}

	desk := &Desk{
		paperTransport: paperTransport,
		liveTransport:  transports.Live,
		paperBalance:   balances.Paper,
		liveBalance:    balances.Live,
		price:          price,
		papers:         NewCache(ctx),
		reals:          NewCache(ctx),
		virtuals:       NewCache(ctx),
		shadows:        &sync.Map{},
		adaptive:       NewAdaptiveRails(),
		virtualArchive: &sync.Map{},
		pending:        &sync.Map{},
	}

	desk.System = runtime.NewSystem(ctx, "desk", desk)

	desk.history = NewHistory(
		ctx,
		price,
		desk.papers,
		desk.reals,
		desk.virtuals,
		paperTransport,
		transports.Live,
		desk.onPositionSettled,
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
	lots := append(
		append(
			desk.papers.AllPositions(),
			desk.reals.AllPositions()...,
		),
		desk.virtuals.AllPositions()...,
	)
	lots = append(lots, desk.shadowPositions()...)
	desk.publishPositions("snapshot", lots...)
	desk.publishBalances()
}

func (desk *Desk) publishBalances() {
	if ui.UI == nil || !ui.UI.Active() {
		return
	}

	frame := map[string]any{}

	if desk.paperBalance != nil {
		if reading := desk.paperBalance.Reading(); reading != nil {
			frame["paper"] = reading
		}
	}

	if desk.liveBalance != nil {
		if reading := desk.liveBalance.Reading(); reading != nil {
			frame["real"] = reading
		}
	}

	if len(frame) == 0 {
		return
	}

	ui.UI.WriteJSON(map[string]any{
		"balances": frame,
	})

	// Keep legacy single-balance frame on the paper wallet for older clients.
	if desk.paperBalance != nil {
		desk.paperBalance.Publish()
	} else if desk.liveBalance != nil {
		desk.liveBalance.Publish()
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
Price is the desk's book-pricing collaborator (spread, mover, tape, allocate).
*/
func (desk *Desk) Price() *Price {
	if desk == nil {
		return nil
	}

	return desk.price
}

/*
OpenPositions returns the open positions in the desk.
*/
func (desk *Desk) OpenPositions() []*Position {
	out := make([]*Position, 0)

	for _, cache := range desk.caches() {
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
	return desk.virtuals.AllPositions()
}

/*
Positions returns every open lot for a symbol across all books.
*/
func (desk *Desk) Positions(symbol string) []*Position {
	if desk == nil || symbol == "" {
		return nil
	}

	out := make([]*Position, 0, 3)

	for _, cache := range desk.caches() {
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
	if position := desk.papers.Get(symbol, true); position != nil {
		return position
	}

	if position := desk.reals.Get(symbol, true); position != nil {
		return position
	}

	return desk.virtuals.Get(symbol, true)
}

/*
PositionOn returns the open lot on one book.
*/
func (desk *Desk) PositionOn(symbol string, ledger Ledger) *Position {
	return desk.cache(ledger).Get(symbol, true)
}

/*
ClosedVirtuals returns virtual lots settled this tick and still on the closed
side of the live cache, awaiting promotion scoring.
*/
func (desk *Desk) ClosedVirtuals() []*Position {
	if desk == nil {
		return nil
	}

	return desk.virtuals.Closed()
}

func (desk *Desk) onPositionSettled(position *Position) {
	if position == nil {
		return
	}

	book := position.Ledger()
	desk.pending.Store(book.PendingKey(position.Symbol), position)

	cache := desk.cache(book)

	cache.Remove(position.Symbol)

	if desk.history != nil {
		if err := desk.history.Store(
			book.HistoryKey()+"/closed/"+position.Symbol+"/"+position.ID,
			position,
		); err != nil {
			errnie.Error(err)
		}
	}

	// Learning shadow keeps receiving marks after the active structural settle.
	if position.ShadowLive() {
		position.onShadowDone = desk.onShadowDone
		desk.shadows.Store(position.ID, position)
		desk.publishPositions("update", position)
		return
	}

	if position.ShadowExitAt != nil {
		desk.persistShadow(position)
	}
}

/*
Refresh reprices open positions against the current book without a trade.
*/
func (desk *Desk) Refresh(symbols []string) {
	if len(symbols) == 0 {
		return
	}

	for _, symbol := range symbols {
		for _, position := range desk.Positions(symbol) {
			mark := desk.price.CurrentMark(position.Symbol)

			if mark == nil {
				continue
			}

			position.Update(mark)
			desk.publishPositions("update", position)
		}

		desk.refreshShadows(symbol)
	}
}

/*
Update marks open positions against the trade price.
*/
func (desk *Desk) Update(trades []kraken.TradeData) {
	if desk.price != nil && desk.price.Tape != nil {
		desk.price.Tape.Note(trades)
	}

	updated := make([]*Position, 0, len(trades))

	for _, trade := range trades {
		for _, position := range desk.Positions(trade.Symbol) {
			position.Update(&trade.Price)
			updated = append(updated, position)
		}

		updated = append(updated, desk.tickShadows(trade.Symbol, &trade.Price)...)
	}

	desk.publishPositions("update", updated...)
}

func (desk *Desk) refreshShadows(symbol string) {
	if desk == nil || desk.price == nil {
		return
	}

	mark := desk.price.CurrentMark(symbol)
	if mark == nil {
		return
	}

	updated := desk.tickShadows(symbol, mark)
	if len(updated) > 0 {
		desk.publishPositions("update", updated...)
	}
}

func (desk *Desk) tickShadows(symbol string, mark *decimal.Decimal) []*Position {
	if desk == nil || desk.shadows == nil || mark == nil {
		return nil
	}

	updated := make([]*Position, 0)

	desk.shadows.Range(func(key, value any) bool {
		position, ok := value.(*Position)
		if !ok || position == nil || position.Symbol != symbol {
			return true
		}

		if position.UpdateShadow(mark) {
			desk.onShadowDone(position)
		}

		updated = append(updated, position)
		return true
	})

	return updated
}

func (desk *Desk) onShadowDone(position *Position) {
	if desk == nil || position == nil {
		return
	}

	desk.shadows.Delete(position.ID)
	desk.persistShadow(position)
	desk.publishPositions("update", position)
}

func (desk *Desk) persistShadow(position *Position) {
	if desk == nil || desk.history == nil || position == nil {
		return
	}

	if err := desk.history.Store(
		"virtual/shadow/"+position.Symbol+"/"+position.ID,
		position,
	); err != nil {
		errnie.Error(err)
	}
}

func (desk *Desk) shadowPositions() []*Position {
	if desk == nil || desk.shadows == nil {
		return nil
	}

	out := make([]*Position, 0)

	desk.shadows.Range(func(_, value any) bool {
		if position, ok := value.(*Position); ok && position != nil {
			out = append(out, position)
		}

		return true
	})

	return out
}

/*
Has reports whether the desk holds an open position on any book for the symbol.
*/
func (desk *Desk) Has(symbol string) bool {
	for _, cache := range desk.caches() {
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
	if desk == nil || desk.price == nil || symbol == "" {
		return false
	}

	qty, _ := desk.price.Allocate(symbol)
	return qty != nil && qty.Sign() > 0
}

/*
BookStale reports whether a symbol's book is stale or absent in the price book manager.
*/
func (desk *Desk) BookStale(symbol string) bool {
	if desk == nil || desk.price == nil || desk.price.Books == nil {
		return false
	}

	return desk.price.Books.StaleSymbol(symbol)
}

/*
Apply an execution to the desk.
*/
func (desk *Desk) Apply(executions *kraken.Execution) {
	for _, execution := range executions.Data {
		for _, cache := range desk.caches() {
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

	cache := desk.cache(ledger)

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
		qty, why = desk.price.AllocateFrom(desk.liveBalance, symbol, true)
	default:
		qty, why = desk.price.AllocateFrom(desk.paperBalance, symbol, true)
	}

	if qty == nil || qty.Sign() <= 0 {
		errnie.Warn("[desk.Enter] failed to allocate quantity", "symbol", symbol, "why", why)
		desk.auditEntryBlocked(symbol, "allocate", why, ledger)

		return &EntryDenial{Gate: "allocate", Reason: why}
	}

	bypass := system.Cfg != nil && system.Cfg.Learning != nil &&
		system.Cfg.Learning.ResonanceBypass

	if desk.price != nil && !(ledger.Virtual() && bypass) {
		if phys := CheckEntryPhysical(desk.price, symbol); !phys.Allowed {
			desk.auditEntryBlocked(symbol, "physical", phys.Reason, ledger)

			return &EntryDenial{Gate: "physical", Reason: phys.Reason}
		}
	}

	transport := desk.transportFor(ledger)

	position := NewPositionWithRails(
		desk.Context(),
		symbol,
		path,
		pathFound,
		ledger,
		desk.price,
		transport,
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
	caches := []*Cache{desk.cache(ledger)}

	// Fall back across books if the caller only knew the symbol.
	if ledger == "" {
		caches = desk.caches()
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
	if desk == nil || key == "" {
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

func (desk *Desk) caches() []*Cache {
	return []*Cache{desk.papers, desk.reals, desk.virtuals}
}

func (desk *Desk) cache(ledger Ledger) *Cache {
	switch ledger {
	case LedgerVirtual:
		return desk.virtuals
	case LedgerReal:
		return desk.reals
	default:
		return desk.papers
	}
}

func (desk *Desk) transportFor(ledger Ledger) Transport {
	switch ledger {
	case LedgerReal:
		if desk.liveTransport != nil {
			return desk.liveTransport
		}

		return desk.paperTransport
	case LedgerVirtual:
		return desk.paperTransport
	default:
		return desk.paperTransport
	}
}
