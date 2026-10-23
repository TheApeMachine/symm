package broker

import (
	"context"
	"io"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

/*
Desk owns open positions: it sizes entries from the book (exit capacity
within the slippage budget, flow noise and cash), works them in child market
orders, takes the learned exit, runs the capacity monitor, accumulates venue
fills beside their shadow fills, and reports each completed round trip.
*/
type Desk struct {
	*runtime.System
	mu        sync.Mutex
	transport Transport
	rest      *network.REST
	price     *Price
	balance   *Balance
	history   *History
	positions *Cache
	virtuals  *Cache
	books     *Book
}

func NewDesk(
	ctx context.Context,
	transport Transport,
	price *Price,
	balance *Balance,
	store *store.Client,
) *Desk {
	desk := &Desk{
		transport: transport,
		rest:      network.NewREST(),
		balance:   balance,
		history:   NewHistory(ctx, store),
		price:     price,
		positions: NewCache(ctx),
		virtuals:  NewCache(ctx),
	}

	desk.System = runtime.NewSystem(ctx, "desk", desk)

	for key, val := range map[string]*Cache{
		"position": desk.positions,
		"virtual":  desk.virtuals,
	} {
		for value := range desk.history.Restore(key) {
			/*
				Prefix is "[origin]/[role]/[scope]/[epoch]/[tick]": the
				role records whether the lot was open or closed when it was
				persisted. The scope is the symbol and carries its own
				slash ("BTC/USD"), so the prefix holds more than five
				segments and only the leading role is read. Anything else —
				an empty value, an interrupted object — is not a position
				record and decodes to a ghost keyed by the empty symbol
				that no update can ever mark or close, so it would
				broadcast to every client forever.
			*/
			fields := strings.Split(value.Prefix, "/")
			if len(fields) < 5 || fields[1] != "open" {
				continue
			}

			position := &Position{}
			value.Lookup(position, key)

			if value.Err != nil {
				// A frame that does not decode is already recorded on the
				// value; report it and keep restoring the rest.
				desk.Error(value.Err)
				continue
			}

			/*
				A frame whose positions layer decoded to nothing (absent,
				empty, or a non-object payload) leaves a zero position on
				our hands. Restoring it would key an untradeable ghost by
				the empty symbol that no Update can ever mark or close, so
				it would broadcast to every client forever. Skip it.
			*/
			if position.Symbol == "" {
				continue
			}

			val.Restore(position)

			if key == "virtual" {
				types.SetVirtualOpen(position.Symbol, true)
			}
		}
	}

	desk.Transition(runtime.READY)
	return desk
}

/*
Hello is a callback for the ui.Hub that fires when a new WebSocket
connection is established, and we want to update the client with
some initial data.
*/
func (desk *Desk) Hello() {
	// One snapshot: every lot the desk still holds. Open until Exit, then
	// the closed virtual until Train drops it. An empty list is the snapshot
	// when nothing is open.
	lots := append(desk.positions.All(true), desk.virtuals.All(true)...)
	lots = append(lots, desk.virtuals.Closed()...)

	io.Copy(ui.UI, data.NewValue("positions", "update", "all").Append(
		lots, "positions",
	))

	if desk.balance.value != nil {
		io.Copy(ui.UI, desk.balance.value)
	}
}

/*
Positions returns the open positions in the desk.
*/
func (desk *Desk) Positions(open bool) []*Position {
	if open {
		return desk.positions.All(true)
	}

	return desk.virtuals.All(true)
}

/*
Update marks open positions against the trade price. Virtual lots close only
through the strategy model (regime exit + priced book), not on a desk threshold.
Called from the workspace tick handler — the desk sits off the trade ingress
path, so this is where marks become live.
*/
func (desk *Desk) Update(trades []kraken.TradeData) {
	/*
		Trade ticks are sparse: one carries a handful of symbols, so the marks
		of everything else simply do not appear in this frame. Hello reads the
		whole desk — every position, moved or not — so the frame always carries
		every card and an unchanged position never blinks out and back.
	*/
	traded := make(map[string]struct{}, len(trades))
	for _, trade := range trades {
		traded[trade.Symbol] = struct{}{}
	}

	for _, val := range []*Cache{
		desk.positions, desk.virtuals,
	} {
		for _, trade := range trades {
			position := val.Get(trade.Symbol, true)

			if position == nil {
				continue
			}

			position.Update(&trade.Price)
		}

		for _, position := range val.All(true) {
			if position == nil || position.Symbol == "" {
				continue
			}

			if _, ok := traded[position.Symbol]; ok {
				continue
			}

			mark := desk.price.CurrentMark(position.Symbol)
			if mark != nil {
				position.Update(mark)
			}
		}
	}

	desk.Hello()
}

/*
Has reports whether the desk holds an open position (paper or virtual)
for the symbol.
*/
func (desk *Desk) Has(symbol string, virtual bool) bool {
	if virtual {
		return desk.virtuals.Has(symbol)
	}

	return desk.positions.Has(symbol)
}

/*
Apply an execution to the desk.
*/
func (desk *Desk) Apply(executions *kraken.Execution) {
	for _, execution := range executions.Data {
		position := desk.positions.Get(execution.Symbol, true)

		if position == nil {
			position = desk.positions.Get(execution.Symbol, false)
		}

		if position == nil {
			position = desk.virtuals.Get(execution.Symbol, true)
		}

		if position == nil {
			continue
		}

		position.Apply(execution)

		// A paper lot whose sell filled is fully settled: retire it from
		// the desk so it cannot shadow the next position on the symbol.
		if !position.Virtual && position.ClosedAt != nil {
			desk.positions.Remove(position.Symbol)
		}
	}
}

/*
Enter a new position.
*/
func (desk *Desk) Enter(
	symbol string, path string, virtual bool,
) error {
	if desk.positions.Has(symbol) || desk.virtuals.Has(symbol) {
		return errnie.Err(
			errnie.TooManyRequests,
			"[desk.Enter] position already exists",
			nil,
		)
	}

	position := NewPosition(symbol, path, virtual, desk.price)

	if desk.price.Status() != runtime.READY {
		return errnie.Err(
			errnie.NotFound,
			"[desk.Enter] price is not ready for "+symbol,
			nil,
		)
	}

	cash := desk.balance.cash(desk.balance.Quote)

	// Allocation cap: never commit more than 20% of current cash.
	if cash != nil && cash.Sign() > 0 {
		cash = cash.Mul(decimal.NewFromFloat64(0.2))
	}

	allocation, err := desk.price.AllocateEntry(symbol, cash)

	if err != nil {
		if errnie.IsNotFound(err) {
			return err
		}

		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[desk.Enter] unable to size entry for "+symbol,
			err,
		))
	}

	qty := allocation.Quantity

	if qty == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[desk.Enter] no tradable quantity for "+symbol,
			nil,
		))
	}

	position.Qty = qty

	order, err := position.Enter()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[desk.Enter] unable to enter "+symbol,
			err,
		))
	}

	key := "position"

	if virtual {
		key = "virtual"
	}

	/*
		The desk owns the position from the moment it enters: register it in
		the right cache so ticks mark it and Hello publishes it immediately.
		Without this only a restart (history restore) would ever surface a
		live entry.
	*/
	cache := desk.positions

	if virtual {
		cache = desk.virtuals
	}

	cache.Add(position.Symbol, position)

	if virtual {
		types.SetVirtualOpen(symbol, true)
	}

	if err := desk.history.Store(data.NewValue(
		key, "open", symbol,
	).Append(
		position, "position",
	)); err != nil {
		cache.Drop(position.Symbol)
		if virtual {
			types.SetVirtualOpen(symbol, false)
		}
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[desk.Enter] failed to store position",
			err,
		))
	}

	if err := desk.Send(order, virtual); err != nil {
		cache.Drop(position.Symbol)
		if virtual {
			types.SetVirtualOpen(symbol, false)
		}
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[desk.Enter] failed to send order",
			err,
		))
	}

	// The entry is live: hand the desk's full snapshot to every client now
	// instead of waiting for the next trade tick to trigger a publish.
	desk.Hello()

	return nil
}

/*
Quote sizes an entry the same way Enter does and returns the quantity, the
entry price, and the entry fee. The book has to be able to take the size.
A price that is not ready is an error: the ledger must not record an
excursion it cannot price.
*/
func (desk *Desk) Quote(
	symbol string,
) (qty, entry, fee *decimal.Decimal, err error) {
	if desk == nil || desk.price == nil {
		return nil, nil, nil, errnie.Err(
			errnie.Validation, "[desk.Quote] price unavailable", nil,
		)
	}

	if desk.balance == nil {
		return nil, nil, nil, errnie.Err(
			errnie.Validation, "[desk.Quote] balance unavailable", nil,
		)
	}

	if desk.price.Status() != runtime.READY {
		return nil, nil, nil, errnie.Err(
			errnie.Validation, "[desk.Quote] price is not ready for "+symbol, nil,
		)
	}

	cash := desk.balance.cash(desk.balance.Quote)

	if cash != nil && cash.Sign() > 0 {
		cash = cash.Mul(decimal.NewFromFloat64(0.2))
	}

	allocation, err := desk.price.AllocateEntry(symbol, cash)

	if err != nil {
		return nil, nil, nil, err
	}

	if allocation == nil || allocation.Quantity == nil || allocation.EntryPrice == nil || allocation.EntryFee == nil {
		return nil, nil, nil, errnie.Err(
			errnie.UnprocessableContent,
			"[desk.Quote] incomplete allocation for "+symbol,
			nil,
		)
	}

	return allocation.Quantity, allocation.EntryPrice, allocation.EntryFee, nil
}

/*
Friction is the round-trip cost of the symbol: the relative spread plus
both fee legs. The return is a fraction of price, the same units as
SaleReturn. ok is false when the book or the fee tier is missing.
*/
func (desk *Desk) Friction(symbol string) (float64, bool) {
	if desk == nil || desk.price == nil || symbol == "" {
		return 0, false
	}

	bid, ask := desk.price.Touch(symbol)
	if bid == nil || ask == nil || bid.Sign() <= 0 || ask.Sign() <= 0 || ask.Cmp(bid) <= 0 {
		return 0, false
	}

	mid := bid.Add(ask).Div(decimal.NewFromInt64(2))
	if mid.Sign() <= 0 {
		return 0, false
	}

	fee := desk.price.Fee(symbol)
	if fee == nil || fee.Fee == nil || fee.Fee.Sign() < 0 {
		return 0, false
	}

	hurdle := ask.Sub(bid).Div(mid).Float64() + 2*fee.Fee.Float64()/100
	if hurdle <= 0 {
		return 0, false
	}

	return hurdle, true
}

/*
OpenMark is the fee-net book return at the fill, the baseline a later
print is measured from.
*/
func (desk *Desk) OpenMark(symbol string) (float64, bool) {
	position := desk.Open(symbol)
	if position == nil || position.Qty == nil || position.EntryPrice == nil || position.EntryFee == nil {
		return 0, false
	}

	return desk.SaleReturn(symbol, position.Qty, position.EntryPrice, position.EntryFee)
}

/*
Realize is the fee-net return of selling quantity through the bid book.
ok is false when the book cannot take the quantity. There is no touch
substitute: a print the book would not pay is not a sample.
*/
func (desk *Desk) Realize(
	symbol string, quantity, entry, fee *decimal.Decimal,
) (float64, bool) {
	if desk == nil || desk.price == nil {
		return 0, false
	}

	_, _, ret, ok := desk.price.Exit(symbol, quantity, entry, fee)

	return ret, ok
}

/*
SaleReturn is the fee-net round-trip fraction if the lot were closed now
through the book (Price.Sale), matching virtual CloseOut settlement.
*/
func (desk *Desk) SaleReturn(
	symbol string, quantity, entry, fee *decimal.Decimal,
) (float64, bool) {
	if desk == nil || desk.price == nil {
		return 0, false
	}

	_, _, ret, ok := desk.price.Sale(symbol, quantity, entry, fee)

	return ret, ok
}

/*
Exit a position and report what settled.
*/
func (desk *Desk) Exit(symbol string, path string, virtual bool, triggers ...string) error {
	var position *Position

	if virtual {
		position = desk.virtuals.Get(symbol, true)
	} else {
		position = desk.positions.Get(symbol, true)
	}

	if position == nil {
		return errnie.Err(
			errnie.NotFound,
			"[desk.Exit] position not found",
			nil,
		)
	}

	order := position.Exit(path)

	// The outcome is final the moment the exit is recorded: settle it
	// before the cache move can retire the row.
	if virtual && position.price != nil {
		cost, proceeds, profit, ret, ok := position.price.CloseOut(
			position.Symbol, position.Qty, position.EntryPrice, position.EntryFee,
		)

		if !ok {
			// No price to close out against: keep the virtual open and
			// retry on the next settlement pass.
			position.mu.Lock()
			position.ClosedAt = nil
			position.HoldNs = 0
			position.mu.Unlock()

			return errnie.Err(
				errnie.UnprocessableContent,
				"[desk.Exit] no price to close out "+position.Symbol,
				nil,
			)
		}

		position.mu.Lock()

		position.Cost = cost
		position.Proceeds = proceeds
		position.Profit = profit

		net := ret * 100
		position.Return = &net
		position.NetReturn = &net
		if len(triggers) > 0 {
			position.OutcomeTriggers = append([]string{}, triggers...)
		} else {
			position.OutcomeTriggers = []string{"desk exit"}
		}

		position.mu.Unlock()
	}

	// The exit is final: the position leaves the open cache. A paper
	// position stays addressable on the closed side until the venue's sell
	// executions land (Apply settles the lot wherever the position lives),
	// then history.Move retires it from the desk entirely.
	if virtual {
		types.SetVirtualOpen(position.Symbol, false)
		desk.virtuals.Remove(position.Symbol)
	} else {
		desk.positions.Remove(position.Symbol)
	}

	key := "position"

	if virtual {
		key = "virtual"
	}

	/*
		The exit is final: the open record is retired and the closed one carries the
		outcome. The open records of a symbol live under "[origin]/open/[symbol]/", the
		closed record under the value's own prefix.
	*/
	value := data.NewValue(
		key, "closed", symbol,
	).Append(
		position, "position",
	)

	if err := desk.history.Move(
		value, key+"/open/"+symbol+"/",
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[desk.Exit] failed to store position",
			err,
		))
	}

	if err := desk.Send(order, virtual); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[desk.Exit] failed to send order",
			err,
		))
	}

	desk.Hello()

	return nil
}

/*
Virtual returns the symbol's open virtual, nil when there is none.
*/
func (desk *Desk) Virtual(symbol string) *Position {
	return desk.virtuals.Get(symbol, true)
}

/*
Occupied reports whether the symbol's single position slot is taken, by a
venue lot or a virtual one.
*/
func (desk *Desk) Occupied(symbol string) bool {
	return desk.positions.Has(symbol) || desk.virtuals.Has(symbol)
}

/*
Open returns the symbol's open lot, preferring the venue lot.
*/
func (desk *Desk) Open(symbol string) *Position {
	if position := desk.positions.Get(symbol, true); position != nil {
		return position
	}

	return desk.virtuals.Get(symbol, true)
}

/*
ExitOccupied closes whichever lot is open on the symbol.
*/
func (desk *Desk) ExitOccupied(symbol, path string) error {
	if desk.positions.Has(symbol) {
		return desk.Exit(symbol, path, false)
	}

	if desk.virtuals.Has(symbol) {
		return desk.Exit(symbol, path, true)
	}

	return errnie.Err(
		errnie.NotFound, "[desk.ExitOccupied] position not found", nil,
	)
}

/*
ClosedVirtuals returns the virtuals sitting on the closed side of the cache:
their outcomes are what Train writes into the model. Drop retires them once
settled.
*/
func (desk *Desk) ClosedVirtuals() []*Position {
	return desk.virtuals.Closed()
}

/*
Drop retires a virtual from the desk entirely, open or closed.
*/
func (desk *Desk) Drop(symbol string) {
	types.SetVirtualOpen(symbol, false)
	desk.virtuals.Drop(symbol)
}

/*
Profit reports whether the open position for the symbol is currently
profitable, based on the position's realized return. The second return
value is false when no open position exists for the symbol.
*/
func (desk *Desk) Profit(symbol string, virtual bool) (bool, bool) {
	var position *Position

	if virtual {
		position = desk.virtuals.Get(symbol, true)
	} else {
		position = desk.positions.Get(symbol, true)
	}

	if position == nil || position.Return == nil {
		return false, false
	}

	return *position.Return > 0, true
}

/*
send an order to the exchange.
*/
func (desk *Desk) Send(order *spot.OrderRequest, virtual bool) error {
	if virtual {
		return nil
	}

	buf, err := sonic.Marshal(order)

	if err != nil {
		return errnie.Err(
			errnie.IO,
			"[desk.Send] failed to marshal order",
			err,
		)
	}

	return desk.transport.Write(buf)
}
