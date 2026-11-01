package broker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"

	"github.com/theapemachine/symm/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/ui"
)

/*
Desk owns open positions: it sizes entries from the book (cash budget,
touch-bid carry divisor, and ask walk), works them in child market
orders, takes the learned exit, runs the capacity monitor, accumulates venue
fills beside their shadow fills, and reports each completed round trip.
*/
type Desk struct {
	*runtime.System
	transport      Transport
	price          *Price
	balance        *Balance
	history        *History
	positions      *Cache
	virtuals       *Cache
	virtualArchive *sync.Map
	pending        *sync.Map
}

func NewDesk(
	ctx context.Context,
	transport Transport,
	price *Price,
	balance *Balance,
) *Desk {
	desk := &Desk{
		transport:      transport,
		balance:        balance,
		price:          price,
		positions:      NewCache(ctx),
		virtuals:       NewCache(ctx),
		virtualArchive: &sync.Map{},
		pending:        &sync.Map{},
	}

	desk.System = runtime.NewSystem(ctx, "desk", desk)

	desk.history = NewHistory(
		ctx,
		price,
		desk.positions,
		desk.virtuals,
		transport,
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
	// The snapshot replaces the book. A mark tick publishes one lot at a
	// time and must not reuse this frame, or the page drops every lot that
	// did not move.
	io.Copy(ui.UI, data.NewValue("positions", "snapshot", "all").Append(
		append(
			desk.positions.AllPositions(),
			desk.virtuals.AllPositions()...,
		), "positions",
	))

	if frame := desk.balance.Frame(); frame != nil {
		io.Copy(ui.UI, frame)
	}
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

	for _, cache := range []*Cache{desk.positions, desk.virtuals} {
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

func (desk *Desk) Position(symbol string) *Position {
	position := desk.positions.Get(symbol, true)

	if position == nil {
		position = desk.virtuals.Get(symbol, true)
	}

	return position
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

	desk.pending.Store(position.Symbol, position)

	cache := desk.positions
	key := "position"

	if position.Virtual {
		cache = desk.virtuals
		key = "virtual"
	}

	cache.Remove(position.Symbol)

	if err := desk.history.Store(data.NewValue(
		key, "closed", position.Symbol,
	).Append(
		position, "position",
	)); err != nil {
		errnie.Error(err)
	}
}

/*
Refresh reprices open positions against the current book without a trade.
Called from the Level 3 ingress so the stoploss sees book changes (exit
VWAP slippage, depth pulls) between volume-clock ticks. Without this, the
evaporation check only fires on the next trade for that symbol, which can
be minutes away on a thin alt.
*/
func (desk *Desk) Refresh(symbols []string) {
	if len(symbols) == 0 {
		return
	}

	for _, symbol := range symbols {
		var position *Position

		if position = desk.positions.Get(symbol, true); position == nil {
			position = desk.virtuals.Get(symbol, true)
		}

		if position == nil {
			continue
		}

		mark := desk.price.CurrentMark(position.Symbol)

		if mark == nil {
			continue
		}

		position.Update(mark)

		scope := position.ID

		if scope == "" {
			scope = position.Symbol

			if position.Virtual {
				scope = "virtual:" + scope
			}
		}

		io.Copy(ui.UI, data.NewValue("positions", "update", scope).Append(
			position, "position",
		))
	}
}

/*
Update marks open positions against the trade price. Called from the workspace
tick handler — the desk sits off the trade ingress path, so this is where
marks become live.

The trade batch is the changed set. Those lots are the ones published. Open lots
whose book mark actually moved are also updated so exits see a live mark.
Unchanged, non-traded lots are skipped to prevent false momentum stalls.
*/
func (desk *Desk) Update(trades []kraken.TradeData) {
	if desk.price != nil && desk.price.Tape != nil {
		desk.price.Tape.Note(trades)
	}

	for _, trade := range trades {
		var position *Position

		if position = desk.positions.Get(trade.Symbol, true); position != nil {
			position.Update(&trade.Price)
		}

		if position = desk.virtuals.Get(trade.Symbol, true); position != nil {
			position.Update(&trade.Price)
		}

		if position == nil {
			continue
		}

		scope := position.ID

		if scope == "" {
			scope = trade.Symbol
			if position.Virtual {
				scope = "virtual:" + scope
			}
		}

		io.Copy(ui.UI, data.NewValue("positions", "update", scope).Append(
			position, "position",
		))
	}
}

/*
Has reports whether the desk holds an open position (paper or virtual)
for the symbol.
*/
func (desk *Desk) Has(symbol string) bool {
	return desk.positions.Has(symbol) || desk.virtuals.Has(symbol)
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
		for _, cache := range []*Cache{desk.positions, desk.virtuals} {
			position := cache.Get(execution.Symbol, true)

			if position == nil {
				continue
			}

			position.Apply(execution)
		}
	}
}

/*
Enter opens a lot on one book. The venue book and the virtual book are
independent: a learning sample does not occupy the venue slot, and a
venue lot does not occupy the learning slot. A second lot on the same
book is rejected.
*/
func (desk *Desk) Enter(
	symbol string, path string, virtual bool, pathFound bool,
) error {
	for key, cache := range map[string]*Cache{
		"position": desk.positions,
		"virtual":  desk.virtuals,
	} {
		if cache.Has(symbol) {
			desk.auditEntryBlocked(symbol, "slot", key+" already exists", virtual)

			return errnie.Err(
				errnie.TooManyRequests,
				"[desk.Enter] "+key+" already exists",
				nil,
			)
		}
	}

	cache := desk.positions
	key := "position"

	if virtual {
		cache = desk.virtuals
		key = "virtual"
	}

	var (
		qty *decimal.Decimal
		why string
	)

	if virtual {
		qty, why = desk.price.AllocateVirtual(symbol)
	} else {
		qty, why = desk.price.Allocate(symbol)
	}

	if qty == nil || qty.Sign() <= 0 {
		errnie.Warn("[desk.Enter] failed to allocate quantity", "symbol", symbol, "why", why)
		desk.auditEntryBlocked(symbol, "allocate", why, virtual)

		return errnie.Err(
			errnie.NotFound,
			fmt.Sprintf(
				"[desk.Enter] failed to allocate quantity for %s: %s",
				symbol, why,
			),
			nil,
		)
	}

	if desk.price != nil {
		if phys := CheckEntryPhysical(desk.price, symbol); !phys.Allowed {
			desk.auditEntryBlocked(symbol, "physical", phys.Reason, virtual)

			return errnie.Err(
				errnie.Validation,
				"[desk.Enter] "+phys.Reason+" for "+symbol,
				nil,
			)
		}
	}

	position := NewPosition(
		desk.Context(),
		symbol,
		path,
		pathFound,
		virtual,
		desk.price,
		desk.transport,
		desk.onPositionSettled,
	)

	// Register before Send. Paper publish blocks until the private ingress
	// reads the fill, and that read calls Apply. The lot has to already be
	// in the cache or the entry price is dropped and close-now PnL stays empty.
	cache.Add(position.Symbol, position, true)

	if err := position.Enter(path, qty); err != nil {
		cache.Drop(position.Symbol)
		desk.auditEntryBlocked(symbol, "send", err.Error(), virtual)
		return err
	}

	if err := desk.history.Store(data.NewValue(
		key, "open", symbol,
	).Append(
		position, "position",
	)); err != nil {
		cache.Drop(position.Symbol)

		return errnie.Error(errnie.Err(
			errnie.IO,
			"[desk.Enter] failed to store position",
			err,
		))
	}

	// The entry is live. Publish this lot; the rest of the book is already
	// on the page from the last snapshot or its own updates.
	io.Copy(ui.UI, data.NewValue("positions", "update", position.ID).Append(
		position, "position",
	))

	return nil
}

func (desk *Desk) auditEntryBlocked(symbol, gate, reason string, virtual bool) {
	audit.Record.Write(map[string]any{
		"node":    "entry",
		"phase":   "blocked",
		"symbol":  symbol,
		"gate":    gate,
		"reason":  reason,
		"virtual": virtual,
	})
}

/*
Exit a position and report what settled.
*/
func (desk *Desk) Exit(
	symbol string, path string, virtual bool,
) error {
	caches := []*Cache{desk.positions, desk.virtuals}

	if virtual {
		caches = []*Cache{desk.virtuals, desk.positions}
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

		// finishExit settles stop closes itself. Policy/UI closes land here.
		if !already && position.IsClosed() {
			desk.onPositionSettled(position)
		}

		return nil
	}

	return nil
}

func (desk *Desk) TakePending(symbol string) (*Position, bool) {
	if desk == nil || symbol == "" {
		return nil, false
	}

	val, ok := desk.pending.LoadAndDelete(symbol)

	if !ok {
		return nil, false
	}

	position, ok := val.(*Position)
	return position, ok && position != nil
}

/*
Pending returns the pending positions in the desk.
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
